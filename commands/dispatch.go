package commands

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"whatsrook/font"
	waSender "whatsrook/sender"
	"whatsrook/store/sqlstore"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	// DefaultPrefix is used when no prefix has been configured in the DB.
	DefaultPrefix = "."
	// PrefixSettingKey is the bot_settings key that stores the prefix list.
	PrefixSettingKey = "prefix"
)

var tablesInitOnce sync.Once

func initTables(ctx context.Context, s *sqlstore.SQLStore) {
	tablesInitOnce.Do(func() {
		db := s.GetDB()
		if db == nil {
			return
		}
		// Create bot_filters table
		_, _ = db.Exec(ctx, `CREATE TABLE IF NOT EXISTS bot_filters (
			our_jid TEXT,
			trigger_word TEXT,
			message_proto TEXT,
			PRIMARY KEY (our_jid, trigger_word)
		)`)

		// Create bot_bgm table
		_, _ = db.Exec(ctx, `CREATE TABLE IF NOT EXISTS bot_bgm (
			our_jid TEXT,
			trigger_word TEXT,
			message_proto TEXT,
			PRIMARY KEY (our_jid, trigger_word)
		)`)

		// Create group_stats table
		_, _ = db.Exec(ctx, `CREATE TABLE IF NOT EXISTS group_stats (
			group_jid TEXT,
			user_jid TEXT,
			date_str TEXT,
			msg_count INTEGER,
			PRIMARY KEY (group_jid, user_jid, date_str)
		)`)

		// Create bot_sticker_cmds table
		_, _ = db.Exec(ctx, `CREATE TABLE IF NOT EXISTS bot_sticker_cmds (
			our_jid TEXT,
			sticker_sha256 TEXT,
			command_name TEXT,
			PRIMARY KEY (our_jid, sticker_sha256)
		)`)
	})
}

// Dispatch checks if the message text is a recognised command and runs it.
// Returns true if a command matched (and was handled), false otherwise.
func Dispatch(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	chatStr := evt.Info.Chat.String()
	senderStr := evt.Info.Sender.String()
	text := extractText(evt)
	slog.Info("Incoming message received", "chat", chatStr, "sender", senderStr, "is_from_me", evt.Info.IsFromMe, "text", text)

	s, okStore := client.Store.Identities.(*sqlstore.SQLStore)
	if okStore {
		initTables(ctx, s)
		if fontStyle, err := s.GetSetting(ctx, "font_style"); err == nil && fontStyle != "" {
			font.SetStyle(fontStyle)
		}
	}

	// 0. Sticker message command trigger
	if evt.Message.StickerMessage != nil {
		if handleStickerCommand(ctx, client, evt) {
			return true
		}
	}

	// 1. Log group message activity
	if evt.Info.Chat.Server == "g.us" {
		slog.Debug("Processing group message", "chat", chatStr, "sender", senderStr)
		logGroupMessage(ctx, client, evt.Info.Chat, evt.Info.Sender)
	}

	// 2. Auto Status Save
	if evt.Info.Chat.String() == "status@broadcast" {
		if okStore {
			raw, _ := s.GetSetting(ctx, "autostatussave")
			if raw == "on" && client.Store.ID != nil {
				ownerJID := client.Store.ID.ToNonAD()
				_, _ = client.SendMessage(ctx, ownerJID, evt.Message)
			}
		}
	}

	// 3. Auto ViewOnce Forwarding
	isViewOnce := false
	if evt.Message.ViewOnceMessage != nil || evt.Message.ViewOnceMessageV2 != nil || evt.Message.ViewOnceMessageV2Extension != nil {
		isViewOnce = true
	} else if img := evt.Message.GetImageMessage(); img != nil && img.GetViewOnce() {
		isViewOnce = true
	} else if vid := evt.Message.GetVideoMessage(); vid != nil && vid.GetViewOnce() {
		isViewOnce = true
	}

	if isViewOnce && okStore {
		raw, _ := s.GetSetting(ctx, "autovv")
		if raw == "on" && client.Store.ID != nil {
			ownerJID := client.Store.ID.ToNonAD()
			unwrapped := waSender.ExtractViewOnceMessage(evt.Message)
			if unwrapped != nil {
				_, _ = client.SendMessage(ctx, ownerJID, unwrapped)
			}
		}
	}

	// Auto Mention Response
	if isBotMentioned(client, evt) && okStore {
		db := s.GetDB()
		if db != nil {
			var mentionProto string
			err := db.QueryRow(ctx, `SELECT value FROM bot_settings WHERE our_jid=$1 AND key='mention_proto'`, client.Store.ID.ToNonAD().String()).Scan(&mentionProto)
			if err == nil && mentionProto != "" {
				if msg, err := waSender.DecodeProtoMessage(mentionProto); err == nil {
					setReplyContextInfo(msg, evt)
					_ = client.SendChatPresence(ctx, evt.Info.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
					time.Sleep(3 * time.Second)
					_, _ = client.SendMessage(ctx, evt.Info.Chat, msg)
					return true
				}
			}
		}
	}

	if text == "" {
		return false
	}

	// 4. Group moderation (anti-link / anti-word)
	if handleGroupModeration(ctx, client, evt, text) {
		return true
	}

	// 5. Check BGM / general filters (auto-response)
	if handleFiltersAndBGM(ctx, client, evt, text) {
		return true
	}

	prefixes := activePrefixes(ctx, client)
	slog.Debug("Checking active prefixes", "prefixes", prefixes, "text", text)
	hasEmpty := false

	// Try non-empty prefixes first.
	for _, p := range prefixes {
		if p == "" {
			hasEmpty = true
			continue
		}
		if strings.HasPrefix(text, p) {
			body := strings.TrimSpace(text[len(p):])
			slog.Info("Prefix matched, executing command", "prefix", p, "body", body)
			return runCommand(ctx, client, evt, body)
		}
	}

	// Empty prefix: treat the whole message as a potential command.
	if hasEmpty {
		body := strings.TrimSpace(text)
		fields := strings.Fields(body)
		if len(fields) > 0 {
			first := fields[0]
			// 1. Direct match without prefix
			if _, exists := Get(strings.ToLower(first)); exists {
				slog.Info("Direct command matched (empty prefix)", "command", first, "body", body)
				return runCommand(ctx, client, evt, body)
			}
			// 2. Match with database configured active prefixes
			for _, p := range activePrefixes(ctx, client) {
				if p != "" && strings.HasPrefix(first, p) {
					strippedName := first[len(p):]
					if _, exists := Get(strings.ToLower(strippedName)); exists {
						strippedBody := strings.TrimSpace(body[len(p):])
						slog.Info("Configured prefix matched", "prefix", p, "command", strippedName, "body", strippedBody)
						return runCommand(ctx, client, evt, strippedBody)
					}
				}
			}
		}
	}

	slog.Debug("No command prefix matched", "text", text)

	if okStore {
		autoAIVal, _ := s.GetSetting(ctx, "autoai:"+chatStr)
		if autoAIVal == "on" && isBotTaggedOrReplied(client, evt, text) {
			slog.Info("AutoAI triggered by tag/reply", "chat", chatStr, "sender", senderStr)
			cctx := &Context{
				Ctx:     ctx,
				Client:  client,
				Evt:     evt,
				Command: "ai",
				Args:    strings.Fields(text),
				RawArgs: text,
				Chat:    evt.Info.Chat,
				Sender:  evt.Info.Sender,
			}
			go func() {
				if cmd, ok := Get("ai"); ok {
					if err := cmd.Handler(cctx); err != nil {
						slog.Error("AutoAI command handler failed", "err", err)
					}
				}
			}()
			return true
		}
	}

	return false
}

// activePrefixes returns the effective prefix list for this session.
// It reads from the DB on every call; for a personal bot the single-row
// query is negligible. Falls back to DefaultPrefix on any error.
func activePrefixes(ctx context.Context, client *whatsmeow.Client) []string {
	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return []string{DefaultPrefix}
	}
	raw, err := s.GetSetting(ctx, PrefixSettingKey)
	if err != nil || raw == "" {
		return []string{DefaultPrefix}
	}
	parts := strings.Fields(raw)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.EqualFold(p, "none") || strings.EqualFold(p, "empty") {
			out = append(out, "") // "none"/"empty" → empty prefix
		} else {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{DefaultPrefix}
	}
	return out
}

// runCommand parses body (prefix already stripped) and executes the matching
// command in a goroutine. Returns false if no command matched.
func runCommand(ctx context.Context, client *whatsmeow.Client, evt *events.Message, body string) bool {
	if body == "" {
		slog.Debug("Empty command body, skipping execution", "chat", evt.Info.Chat.String())
		return false
	}
	if isSenderBanned(ctx, client, evt.Info.Sender) {
		slog.Warn("Sender is banned, ignoring command", "sender", evt.Info.Sender.String(), "chat", evt.Info.Chat.String())
		return false
	}

	fields := strings.Fields(body)
	name := strings.ToLower(fields[0])
	args := fields[1:]

	cmd, ok := Get(name)
	if !ok {
		slog.Debug("Command not found", "name", name, "chat", evt.Info.Chat.String())
		return false
	}

	rawArgs := ""
	if idx := strings.Index(body, fields[0]); idx == 0 {
		rawArgs = strings.TrimSpace(body[len(fields[0]):])
	}

	// If no arguments are provided, and this is a reply to another message,
	// treat the quoted message text/caption as the arguments.
	if len(args) == 0 {
		if quoted := getQuotedMessageFromEvent(evt); quoted != nil {
			if quotedText := extractTextFromProto(quoted); quotedText != "" {
				args = strings.Fields(quotedText)
				rawArgs = quotedText
			}
		}
	}

	cctx := &Context{
		Ctx:     ctx,
		Client:  client,
		Evt:     evt,
		Command: name,
		Args:    args,
		RawArgs: rawArgs,
		Chat:    evt.Info.Chat,
		Sender:  evt.Info.Sender,
	}

	s, okSetting := client.Store.Identities.(*sqlstore.SQLStore)

	go func() {
		// 1. Group-only check
		if cmd.GroupOnly && cctx.Chat.Server != "g.us" {
			slog.Warn("Group-only command executed in non-group chat JID", "command", name, "chat", cctx.Chat.String())
			_ = cctx.Reply("This command can only be used in a group chat.")
			return
		}

		// 2. Public vs Sudo check
		if okSetting {
			botMode, _ := s.GetSetting(ctx, "mode")
			if botMode == "private" && !cctx.IsSudo() {
				slog.Warn("Private mode check failed", "command", name, "sender", cctx.Sender.String())
				_ = cctx.Reply("The bot is currently in private mode. Only sudoers/owners can use it.")
				return
			}
		}

		if !cmd.IsPublic && !cctx.IsSudo() {
			slog.Warn("Sudoer command check failed", "command", name, "sender", cctx.Sender.String())
			_ = cctx.Reply("This command is restricted to sudoers/owners only.")
			return
		}

		// 3. Disabled check
		if okSetting {
			raw, _ := s.GetSetting(ctx, "disabled_commands")
			if raw != "" {
				isDisabled := false
				for disabled := range strings.FieldsSeq(raw) {
					if strings.EqualFold(disabled, name) {
						isDisabled = true
						break
					}
				}
				if isDisabled {
					slog.Warn("Disabled command check failed", "command", name)
					_ = cctx.Reply(fmt.Sprintf(" Command %q is currently disabled.", name))
					return
				}
			}
		}

		slog.Info("Executing command", "command", name, "chat", cctx.Chat.String(), "sender", cctx.Sender.String(), "args", cctx.Args)
		if err := cmd.Handler(cctx); err != nil {
			slog.Error("Command handler failed", "command", name, "err", err)
			logHandlerErr(name, err)
		} else {
			slog.Info("Command completed successfully", "command", name)
		}
	}()

	return true
}

func extractText(evt *events.Message) string {
	if evt.Message.GetConversation() != "" {
		return evt.Message.GetConversation()
	}
	if evt.Message.GetExtendedTextMessage() != nil {
		return evt.Message.GetExtendedTextMessage().GetText()
	}
	if btnResp := evt.Message.GetButtonsResponseMessage(); btnResp != nil {
		if id := btnResp.GetSelectedButtonID(); id != "" {
			return id
		}
		return btnResp.GetSelectedDisplayText()
	}
	if templateResp := evt.Message.GetTemplateButtonReplyMessage(); templateResp != nil {
		if id := templateResp.GetSelectedID(); id != "" {
			return id
		}
		return templateResp.GetSelectedDisplayText()
	}
	if interactiveResp := evt.Message.GetInteractiveResponseMessage(); interactiveResp != nil {
		if nativeFlow := interactiveResp.GetNativeFlowResponseMessage(); nativeFlow != nil {
			if params := nativeFlow.GetParamsJSON(); params != "" {
				return params
			}
		}
		if body := interactiveResp.GetBody(); body != nil {
			return body.GetText()
		}
	}
	if listResp := evt.Message.GetListResponseMessage(); listResp != nil {
		if singleSelect := listResp.GetSingleSelectReply(); singleSelect != nil {
			return singleSelect.GetSelectedRowID()
		}
	}
	return ""
}

func isSenderBanned(ctx context.Context, client *whatsmeow.Client, sender types.JID) bool {
	if client.Store.ID == nil {
		return false
	}
	ownerJID := client.Store.ID.ToNonAD()
	senderJID := sender.ToNonAD()
	if senderJID == ownerJID {
		return false
	}

	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return false
	}

	rawSudo, _ := s.GetSetting(ctx, "sudoers")
	for sudoerStr := range strings.FieldsSeq(rawSudo) {
		sudoerJID, err := types.ParseJID(sudoerStr)
		if err == nil {
			if senderJID == sudoerJID.ToNonAD() {
				return false
			}
		}
	}

	rawBanned, _ := s.GetSetting(ctx, "banned_users")
	for bannedStr := range strings.FieldsSeq(rawBanned) {
		bannedJID, err := types.ParseJID(bannedStr)
		if err == nil {
			if senderJID == bannedJID.ToNonAD() {
				return true
			}
		}
	}

	return false
}

func setReplyContextInfo(msg *waE2E.Message, evt *events.Message) {
	stanzaID := evt.Info.ID
	participant := evt.Info.Sender.ToNonAD().String()
	ci := &waE2E.ContextInfo{
		StanzaID:      &stanzaID,
		Participant:   &participant,
		QuotedMessage: evt.Message,
	}

	if msg.ExtendedTextMessage != nil {
		msg.ExtendedTextMessage.ContextInfo = ci
	} else if msg.ImageMessage != nil {
		msg.ImageMessage.ContextInfo = ci
	} else if msg.VideoMessage != nil {
		msg.VideoMessage.ContextInfo = ci
	} else if msg.AudioMessage != nil {
		msg.AudioMessage.ContextInfo = ci
	} else if msg.DocumentMessage != nil {
		msg.DocumentMessage.ContextInfo = ci
	} else if msg.StickerMessage != nil {
		msg.StickerMessage.ContextInfo = ci
	}
}

func logGroupMessage(ctx context.Context, client *whatsmeow.Client, chat, sender types.JID) {
	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return
	}
	initTables(ctx, s)
	db := s.GetDB()
	if db == nil {
		return
	}
	dateStr := time.Now().Format("2006-01-02")
	query := `
		INSERT INTO group_stats (group_jid, user_jid, date_str, msg_count)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT(group_jid, user_jid, date_str) DO UPDATE SET msg_count = group_stats.msg_count + 1
	`
	_, _ = db.Exec(ctx, query, chat.String(), sender.ToNonAD().String(), dateStr)
}

func handleFiltersAndBGM(ctx context.Context, client *whatsmeow.Client, evt *events.Message, text string) bool {
	if evt.Info.Chat.Server == "g.us" {
		return false
	}
	if isSenderBanned(ctx, client, evt.Info.Sender) {
		return false
	}
	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return false
	}
	db := s.GetDB()
	if db == nil {
		return false
	}

	ourJID := client.Store.ID.ToNonAD().String()
	trigger := strings.TrimSpace(strings.ToLower(text))

	// 1. Check BGM first
	var bgmProto string
	err := db.QueryRow(ctx, `SELECT message_proto FROM bot_bgm WHERE our_jid=$1 AND trigger_word=$2`, ourJID, trigger).Scan(&bgmProto)
	if err == nil && bgmProto != "" {
		if msg, err := waSender.DecodeProtoMessage(bgmProto); err == nil {
			setReplyContextInfo(msg, evt)
			_ = client.SendChatPresence(ctx, evt.Info.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaAudio)
			time.Sleep(3 * time.Second)
			_, _ = client.SendMessage(ctx, evt.Info.Chat, msg)
			return true
		}
	}

	// 2. Check general filters
	var filterProto string
	err = db.QueryRow(ctx, `SELECT message_proto FROM bot_filters WHERE our_jid=$1 AND trigger_word=$2`, ourJID, trigger).Scan(&filterProto)
	if err == nil && filterProto != "" {
		if msg, err := waSender.DecodeProtoMessage(filterProto); err == nil {
			setReplyContextInfo(msg, evt)
			_ = client.SendChatPresence(ctx, evt.Info.Chat, types.ChatPresenceComposing, types.ChatPresenceMediaText)
			time.Sleep(3 * time.Second)
			_, _ = client.SendMessage(ctx, evt.Info.Chat, msg)
			return true
		}
	}

	return false
}

func handleGroupModeration(ctx context.Context, client *whatsmeow.Client, evt *events.Message, text string) bool {
	if evt.Info.Chat.Server != "g.us" {
		return false
	}
	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return false
	}

	chatStr := evt.Info.Chat.String()
	sender := evt.Info.Sender.ToNonAD()

	// Check if antilink is enabled
	antiLinkEnabled := false
	rawLink, _ := s.GetSetting(ctx, "antilink:"+chatStr)
	if rawLink == "on" {
		antiLinkEnabled = true
	}

	// Check if antiword is configured
	var bannedWords []string
	rawWord, _ := s.GetSetting(ctx, "antiword:"+chatStr)
	if rawWord != "" {
		bannedWords = strings.Fields(strings.ToLower(rawWord))
	}

	if !antiLinkEnabled && len(bannedWords) == 0 {
		return false
	}

	// Check if sender is admin
	info, err := client.GetGroupInfo(ctx, evt.Info.Chat)
	if err != nil {
		return false
	}

	if waSender.IsAdminRaw(ctx, client, info, sender) {
		return false
	}

	violation := false
	reason := ""

	if antiLinkEnabled {
		lowerText := strings.ToLower(text)
		if strings.Contains(lowerText, "http://") || strings.Contains(lowerText, "https://") || strings.Contains(lowerText, "www.") || strings.Contains(lowerText, ".com") || strings.Contains(lowerText, ".net") || strings.Contains(lowerText, ".org") {
			violation = true
			reason = "links"
		}
	}

	if !violation && len(bannedWords) > 0 {
		lowerText := strings.ToLower(text)
		for _, w := range bannedWords {
			if strings.Contains(lowerText, w) {
				violation = true
				reason = "banned words"
				break
			}
		}
	}

	if violation {
		botIsAdmin := false
		if client.Store.ID != nil {
			botIsAdmin = waSender.IsAdminRaw(ctx, client, info, *client.Store.ID)
		}

		if botIsAdmin {
			_, _ = client.SendMessage(ctx, evt.Info.Chat, client.BuildRevoke(evt.Info.Chat, evt.Info.Sender, evt.Info.ID))
			resolvedJID, username := waSender.ResolveMentionRaw(ctx, client, evt.Info.Sender)
			textMsg := fmt.Sprintf(" Message from @%s deleted: contains %s.", username, reason)
			_, _ = client.SendMessage(ctx, evt.Info.Chat, &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text: &textMsg,
					ContextInfo: &waE2E.ContextInfo{
						MentionedJID: []string{resolvedJID.ToNonAD().String()},
					},
				},
			})
			return true
		}
	}

	return false
}

func isBotMentioned(client *whatsmeow.Client, evt *events.Message) bool {
	if client.Store.ID == nil {
		return false
	}
	ourJID := client.Store.ID.ToNonAD()

	var mentions []string
	if ext := evt.Message.GetExtendedTextMessage(); ext != nil {
		if ci := ext.GetContextInfo(); ci != nil {
			mentions = ci.MentionedJID
		}
	}

	ourLID := ourJID
	if ourJID.Server == types.DefaultUserServer && client.Store.LIDs != nil {
		if lid, err := client.Store.LIDs.GetLIDForPN(context.Background(), ourJID); err == nil && !lid.IsEmpty() {
			ourLID = lid.ToNonAD()
		}
	} else if ourJID.Server == types.HiddenUserServer && client.Store.LIDs != nil {
		if pn, err := client.Store.LIDs.GetPNForLID(context.Background(), ourJID); err == nil && !pn.IsEmpty() {
			ourLID = pn.ToNonAD()
		}
	}

	for _, m := range mentions {
		mj, err := types.ParseJID(m)
		if err == nil {
			mj = mj.ToNonAD()
			if mj == ourJID || mj == ourLID {
				return true
			}
		}
	}
	return false
}

func handleStickerCommand(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	stk := evt.Message.StickerMessage
	if stk == nil || len(stk.FileSHA256) == 0 {
		return false
	}

	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return false
	}
	db := s.GetDB()
	if db == nil {
		return false
	}

	ourJID := client.Store.ID.ToNonAD().String()
	shaHex := hex.EncodeToString(stk.FileSHA256)

	var cmdName string
	err := db.QueryRow(ctx, `SELECT command_name FROM bot_sticker_cmds WHERE our_jid=$1 AND sticker_sha256=$2`, ourJID, shaHex).Scan(&cmdName)
	if err != nil || cmdName == "" {
		return false
	}

	cmd, exists := Get(cmdName)
	if !exists {
		return false
	}

	var args []string
	var rawArgs string

	if ext := evt.Message.GetExtendedTextMessage(); ext != nil {
		if ci := ext.GetContextInfo(); ci != nil && ci.QuotedMessage != nil {
			quotedText := extractTextFromProto(ci.QuotedMessage)
			if quotedText != "" {
				args = strings.Fields(quotedText)
				rawArgs = quotedText
			}
		}
	} else if ci := stk.GetContextInfo(); ci != nil && ci.QuotedMessage != nil {
		quotedText := extractTextFromProto(ci.QuotedMessage)
		if quotedText != "" {
			args = strings.Fields(quotedText)
			rawArgs = quotedText
		}
	}

	cctx := &Context{
		Ctx:     ctx,
		Client:  client,
		Evt:     evt,
		Command: cmdName,
		Args:    args,
		RawArgs: rawArgs,
		Chat:    evt.Info.Chat,
		Sender:  evt.Info.Sender,
	}

	go func() {
		botMode, _ := s.GetSetting(ctx, "mode")
		if botMode == "private" && !cctx.IsSudo() {
			_ = cctx.Reply("The bot is currently in private mode. Only sudoers/owners can use it.")
			return
		}

		raw, _ := s.GetSetting(ctx, "disabled_commands")
		if raw != "" {
			for disabled := range strings.FieldsSeq(raw) {
				if strings.EqualFold(disabled, cmdName) {
					_ = cctx.Reply(fmt.Sprintf(" Command %q is currently disabled.", cmdName))
					return
				}
			}
		}

		if err := cmd.Handler(cctx); err != nil {
			logHandlerErr(cmdName, err)
		}
	}()

	return true
}

func extractTextFromProto(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if msg.GetConversation() != "" {
		return msg.GetConversation()
	}
	if msg.GetExtendedTextMessage() != nil {
		return msg.GetExtendedTextMessage().GetText()
	}
	if msg.GetImageMessage() != nil && msg.GetImageMessage().GetCaption() != "" {
		return msg.GetImageMessage().GetCaption()
	}
	if msg.GetVideoMessage() != nil && msg.GetVideoMessage().GetCaption() != "" {
		return msg.GetVideoMessage().GetCaption()
	}
	if msg.GetDocumentMessage() != nil && msg.GetDocumentMessage().GetCaption() != "" {
		return msg.GetDocumentMessage().GetCaption()
	}
	return ""
}

func getQuotedMessageFromEvent(evt *events.Message) *waE2E.Message {
	if evt == nil || evt.Message == nil {
		return nil
	}
	var ci *waE2E.ContextInfo
	msg := evt.Message
	if msg.GetExtendedTextMessage() != nil {
		ci = msg.GetExtendedTextMessage().GetContextInfo()
	} else if msg.GetImageMessage() != nil {
		ci = msg.GetImageMessage().GetContextInfo()
	} else if msg.GetVideoMessage() != nil {
		ci = msg.GetVideoMessage().GetContextInfo()
	} else if msg.GetAudioMessage() != nil {
		ci = msg.GetAudioMessage().GetContextInfo()
	} else if msg.GetDocumentMessage() != nil {
		ci = msg.GetDocumentMessage().GetContextInfo()
	}
	if ci != nil {
		return ci.QuotedMessage
	}
	return nil
}

func isBotTaggedOrReplied(client *whatsmeow.Client, evt *events.Message, text string) bool {
	if client.Store.ID == nil {
		return false
	}
	ourJID := client.Store.ID.ToNonAD()
	ourLID := client.Store.LID.ToNonAD()

	// 1. Check if the text itself contains a mention/tag of the bot
	if strings.Contains(text, "@"+ourJID.User) || (!ourLID.IsEmpty() && strings.Contains(text, "@"+ourLID.User)) {
		return true
	}

	var ctxInfo *waE2E.ContextInfo
	if evt.Message.GetExtendedTextMessage() != nil {
		ctxInfo = evt.Message.GetExtendedTextMessage().ContextInfo
	} else if evt.Message.GetImageMessage() != nil {
		ctxInfo = evt.Message.GetImageMessage().ContextInfo
	} else if evt.Message.GetVideoMessage() != nil {
		ctxInfo = evt.Message.GetVideoMessage().ContextInfo
	} else if evt.Message.GetAudioMessage() != nil {
		ctxInfo = evt.Message.GetAudioMessage().ContextInfo
	} else if evt.Message.GetDocumentMessage() != nil {
		ctxInfo = evt.Message.GetDocumentMessage().ContextInfo
	}

	if ctxInfo == nil {
		return false
	}

	// 2. Check if the bot is mentioned/tagged in MentionedJID metadata
	for _, m := range ctxInfo.MentionedJID {
		if parseJID, err := types.ParseJID(m); err == nil {
			nonAD := parseJID.ToNonAD()
			if nonAD == ourJID || (!ourLID.IsEmpty() && nonAD == ourLID) {
				return true
			}
		}
	}

	// 3. Check if the message is a reply/quote to a message sent by the bot
	if ctxInfo.Participant != nil {
		if parseJID, err := types.ParseJID(*ctxInfo.Participant); err == nil {
			nonAD := parseJID.ToNonAD()
			if nonAD == ourJID || (!ourLID.IsEmpty() && nonAD == ourLID) {
				return true
			}
		}
	}

	return false
}
