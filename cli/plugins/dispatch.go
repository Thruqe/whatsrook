// Event dispatcher – parses incoming messages, matches command prefixes, runs
// moderation, and routes commands to their handlers.
package plugins

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	waSender "whatsrook/messaging"
	"whatsrook/utils"
	"whatsrook/wa-core/store/sqlstore"

	"whatsrook/wa-core"
	"whatsrook/wa-core/proto/waE2E"
	"whatsrook/wa-core/types"
	"whatsrook/wa-core/types/events"
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

		// Create bot_user_xp table
		_, _ = db.Exec(ctx, `CREATE TABLE IF NOT EXISTS bot_user_xp (
			user_jid TEXT PRIMARY KEY,
			xp INTEGER DEFAULT 0,
			ttt_wins INTEGER DEFAULT 0,
			ttt_losses INTEGER DEFAULT 0,
			ttt_draws INTEGER DEFAULT 0,
			wcg_wins INTEGER DEFAULT 0,
			wcg_games INTEGER DEFAULT 0,
			wcg_rating INTEGER DEFAULT 1000
		)`)
		_, _ = db.Exec(ctx, `ALTER TABLE bot_user_xp ADD COLUMN wcg_wins INTEGER DEFAULT 0`)
		_, _ = db.Exec(ctx, `ALTER TABLE bot_user_xp ADD COLUMN wcg_games INTEGER DEFAULT 0`)
		_, _ = db.Exec(ctx, `ALTER TABLE bot_user_xp ADD COLUMN wcg_rating INTEGER DEFAULT 1000`)

		// Create bot_group_user_xp table for per-group leaderboards
		_, _ = db.Exec(ctx, `CREATE TABLE IF NOT EXISTS bot_group_user_xp (
			group_jid TEXT NOT NULL,
			user_jid TEXT NOT NULL,
			xp INTEGER DEFAULT 0,
			ttt_wins INTEGER DEFAULT 0,
			ttt_losses INTEGER DEFAULT 0,
			ttt_draws INTEGER DEFAULT 0,
			wcg_wins INTEGER DEFAULT 0,
			wcg_games INTEGER DEFAULT 0,
			wcg_rating INTEGER DEFAULT 1000,
			PRIMARY KEY (group_jid, user_jid)
		)`)
	})
}

// Dispatch checks if the message text is a recognised command and runs it.
// Returns true if a command matched (and was handled), false otherwise.
func Dispatch(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	if utils.IsNetworkPaused() {
		_, reason, _ := utils.GetNetworkStatus()
		slog.Warn("Process is paused due to network issues, skipping command dispatch", "reason", reason)
		return false
	}

	chatStr := evt.Info.Chat.String()
	senderStr := evt.Info.Sender.String()
	text := extractText(evt)
	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		var respJSON struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(text), &respJSON); err == nil && respJSON.ID != "" {
			slog.Debug("Parsed JSON interactive response ID", "original", text, "extracted_id", respJSON.ID)
			text = respJSON.ID
		}
	}
	slog.Debug("Incoming message received", "chat", chatStr, "sender", senderStr, "is_from_me", evt.Info.IsFromMe, "text", text)

	if strings.HasPrefix(text, "cancel_loader_") {
		loaderID := strings.TrimPrefix(text, "cancel_loader_")
		slog.Info("Cancel interactive loader button pressed", "loaderID", loaderID)
		if waSender.CancelLoader(loaderID) {
			return true
		}
	}

	s, okStore := client.Store.Identities.(*sqlstore.SQLStore)
	if okStore {
		initTables(ctx, s)
		StartAutoMuteScheduler(ctx, client)
		StartAutoBioScheduler(ctx, client)
		if fontStyle, err := s.GetSetting(ctx, "font_style"); err == nil && fontStyle != "" {
			utils.SetFontStyle(fontStyle)
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

	// 2. Group moderation (antimsg / antispam / anti-link / anti-word)
	if handleGroupModeration(ctx, client, evt, text) {
		return true
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
	if (evt.IsViewOnce || evt.IsViewOnceV2 || waSender.IsViewOnceMessage(evt.Message)) && okStore {
		raw, _ := s.GetSetting(ctx, "autovv")
		if raw == "on" {
			mode, _ := s.GetSetting(ctx, "autovv_mode")
			var targetJID types.JID
			if (mode == "public" || mode == "chat") && !evt.Info.Chat.IsEmpty() {
				targetJID = evt.Info.Chat
			} else if client.Store.ID != nil {
				targetJID = client.Store.ID.ToNonAD()
			}

			if !targetJID.IsEmpty() {
				go func() {
					err := waSender.UnwrapAndSendViewOnceMessage(context.Background(), client, evt.Message, evt.Info.Sender, evt.Info.PushName, targetJID, evt.Info.Chat)
					if err != nil {
						slog.Error("AutoVV forwarding failed", "chat", evt.Info.Chat.String(), "err", err)
					}
				}()
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
					_, _ = client.SendMessage(ctx, evt.Info.Chat, msg)
					return true
				}
			}
		}
	}

	if text == "" {
		return false
	}

	// 5. Check BGM / general filters (auto-response)
	if handleFiltersAndBGM(ctx, client, evt, text) {
		return true
	}

	prefixes := activePrefixes(ctx, client)
	slog.Debug("Checking active prefixes", "prefixes", prefixes, "text", text)

	// Check if user is currently replying with their chosen new bot name
	if okStore {
		senderUser := evt.Info.Sender.ToNonAD().User
		awaitingInput, _ := s.GetSetting(ctx, BotNameAwaitingInputPrefix+senderUser)
		if awaitingInput == "true" {
			newName := strings.TrimSpace(text)
			if newName != "" {
				_ = s.PutSetting(ctx, BotNameSettingKey, newName)
				_ = s.PutSetting(ctx, BotNamePromptDismissedKey, "true")
				_ = s.PutSetting(ctx, BotNameAwaitingInputPrefix+senderUser, "")

				p := prefixes[0]
				if p == "" {
					p = DefaultPrefix
				}
				cctx := &Context{
					Ctx:    ctx,
					Client: client,
					Evt:    evt,
					Chat:   evt.Info.Chat,
					Sender: evt.Info.Sender,
				}
				_ = cctx.Reply(fmt.Sprintf("Bot name updated successfully to \"*%s*\"! 🎉\n\nYou can change it anytime later using the %sbotname command (e.g. `%sbotname <name>`).", newName, p, p))
				return true
			}
		}
	}

	hasEmpty := false

	// Try non-empty prefixes first.
	for _, p := range prefixes {
		if p == "" {
			hasEmpty = true
			continue
		}
		if matchesPrefix(text, p) {
			body := strings.TrimLeft(strings.TrimSpace(text[len(p):]), ",:;! \t")
			slog.Debug("Prefix matched, executing command", "prefix", p, "body", body)
			if runCommand(ctx, client, evt, body) {
				return true
			}
		}
	}

	// Active Tic-Tac-Toe move listener without prefix (e.g. typing "1", "2", ... "9")
	trimmedText := strings.TrimSpace(text)
	if IsTTTGameActive(chatStr) && len(trimmedText) == 1 && trimmedText >= "1" && trimmedText <= "9" {
		slog.Debug("Direct move matched active Tic-Tac-Toe game", "chat", chatStr, "move", trimmedText)
		return runCommand(ctx, client, evt, "ttt "+trimmedText)
	}

	// Unscramble Game listener — lobby join or active turn
	if utils.GetUnscrambleGame(chatStr) != nil {
		cctx := &Context{
			Ctx:    ctx,
			Client: client,
			Evt:    evt,
			Chat:   evt.Info.Chat,
			Sender: evt.Info.Sender,
		}
		if HandleUnscrambleLobbyInput(cctx, text) {
			return true
		}
		if HandleUnscrambleInput(cctx, text) {
			return true
		}
	}

	// Word Chain Game (WCG) listener — lobby join or active turn
	if utils.GetWCGGame(chatStr) != nil {
		cctx := &Context{
			Ctx:    ctx,
			Client: client,
			Evt:    evt,
			Chat:   evt.Info.Chat,
			Sender: evt.Info.Sender,
		}
		if HandleWCGLobbyInput(cctx, text) {
			return true
		}
		if HandleWCGInput(cctx, text) {
			return true
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
				slog.Debug("Direct command matched (empty prefix)", "command", first, "body", body)
				return runCommand(ctx, client, evt, body)
			}
			// 2. Match with database configured active prefixes
			for _, p := range activePrefixes(ctx, client) {
				if p != "" && strings.HasPrefix(first, p) {
					strippedName := first[len(p):]
					if _, exists := Get(strings.ToLower(strippedName)); exists {
						strippedBody := strings.TrimSpace(body[len(p):])
						slog.Debug("Configured prefix matched", "prefix", p, "command", strippedName, "body", strippedBody)
						return runCommand(ctx, client, evt, strippedBody)
					}
				}
			}
		}
	}

	slog.Debug("No command prefix matched", "text", text)

	if okStore {
		autoAIVal, _ := s.GetSetting(ctx, "autoai:"+chatStr)
		if autoAIVal == "" {
			autoAIVal, _ = s.GetSetting(ctx, "autoai")
		}
		if autoAIVal == "on" && isBotTaggedOrReplied(client, evt, text) {
			slog.Debug("AutoAI triggered by tag/reply/prefix", "chat", chatStr, "sender", senderStr)

			prompt := text
			for _, p := range prefixes {
				if p != "" && matchesPrefix(text, p) {
					prompt = strings.TrimSpace(text[len(p):])
					break
				}
			}

			botName := GetBotName(ctx, client)
			if botName != "" && strings.HasPrefix(strings.ToLower(prompt), strings.ToLower(botName)) {
				prompt = strings.TrimSpace(prompt[len(botName):])
			}

			if prompt == "" {
				prompt = text
			}

			cctx := &Context{
				Ctx:     ctx,
				Client:  client,
				Evt:     evt,
				Command: "ai",
				Args:    strings.Fields(prompt),
				RawArgs: prompt,
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

func matchesPrefix(text, p string) bool {
	if p == "" {
		return false
	}

	lowerText := strings.ToLower(text)
	lowerP := strings.ToLower(p)

	if !strings.HasPrefix(lowerText, lowerP) {
		return false
	}

	if isWordPrefix(p) {
		rem := text[len(p):]
		if len(rem) == 0 {
			return true
		}
		firstRune, _ := utf8.DecodeRuneInString(rem)
		if unicode.IsLetter(firstRune) || unicode.IsNumber(firstRune) {
			return false
		}
	}

	return true
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
		if len(fields) > 1 {
			for i := 1; i < len(fields); i++ {
				subName := strings.ToLower(fields[i])
				if subCmd, subOk := Get(subName); subOk {
					name = subName
					cmd = subCmd
					ok = true
					args = fields[i+1:]
					break
				}
			}
		}
	}
	if !ok {
		slog.Debug("Command not found", "name", name, "chat", evt.Info.Chat.String())
		return false
	}

	// Bot command triggered! Check if default bot name setup prompt needs to be shown.
	s, okSetting := client.Store.Identities.(*sqlstore.SQLStore)
	if okSetting {
		botName := GetBotName(ctx, client)
		if strings.EqualFold(botName, "whatsrook") || strings.EqualFold(botName, "rook") {
			dismissed, _ := s.GetSetting(ctx, BotNamePromptDismissedKey)
			if dismissed != "true" {
				cmdWord := strings.ToLower(name)
				if cmdWord != "botname" && cmdWord != "setbotname" && cmdWord != "setname" && cmdWord != "name" && cmdWord != "setbot" && cmdWord != "reconfigure" && cmdWord != "reconfig" && cmdWord != "setupwizard" {
					cctx := &Context{
						Ctx:    ctx,
						Client: client,
						Evt:    evt,
						Chat:   evt.Info.Chat,
						Sender: evt.Info.Sender,
					}
					p := activePrefixes(ctx, client)[0]
					if p == "" {
						p = DefaultPrefix
					}
					bodyText := "*BOT NAME CUSTOMIZATION RECOMMENDED*\n\nIt's highly recommended to give your own copy of WhatsRook its own name!\nFor example, you can name it something like *Fuzzy* or *Meow*.\n\nYou can also run *" + p + "reconfigure* anytime to open the setup wizard."
					buttons := []struct{ ID, Text string }{
						{ID: p + "setbot setup_customize", Text: "Customize Bot"},
						{ID: p + "setbot setup_continue", Text: "Continue"},
					}
					_ = sendInteractiveButtons(cctx, bodyText, fmt.Sprintf("Powered by %s", botName), buttons)
					return true
				}
			}
		}
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

	go func() {
		reqCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		cctx := &Context{
			Ctx:        reqCtx,
			CancelFunc: cancel,
			Client:     client,
			Evt:        evt,
			Command:    name,
			Args:       args,
			RawArgs:    rawArgs,
			Chat:       evt.Info.Chat,
			Sender:     evt.Info.Sender,
		}
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
				slog.Warn("Private mode check failed - silently ignoring non-sudoer", "command", name, "sender", cctx.Sender.String())
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

		slog.Debug("Executing command", "command", name, "chat", cctx.Chat.String(), "sender", cctx.Sender.String(), "args", cctx.Args)
		if err := cmd.Handler(cctx); err != nil {
			LogHandlerErrWithContext(cctx, name, err)
		} else {
			slog.Debug("Command completed successfully", "command", name)
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
				var respJSON struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal([]byte(params), &respJSON); err == nil && respJSON.ID != "" {
					return respJSON.ID
				}
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
			_, _ = client.SendMessage(ctx, evt.Info.Chat, msg)
			return true
		}
	}

	return false
}

var (
	spamTrackMu sync.Mutex
	spamHistory = make(map[string][]time.Time)
)

func checkSpamLimit(chatStr, senderStr string, maxMsgs int) bool {
	spamTrackMu.Lock()
	defer spamTrackMu.Unlock()

	key := chatStr + ":" + senderStr
	now := time.Now()
	cutoff := now.Add(-5 * time.Second)

	history := spamHistory[key]
	var recent []time.Time
	for _, t := range history {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	spamHistory[key] = recent

	return len(recent) > maxMsgs
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

	// Check if antimsg is enabled and sender is in target list
	rawAntiMsgStatus, _ := s.GetSetting(ctx, "antimsg_status:"+chatStr)
	if rawAntiMsgStatus == "on" {
		rawAntiMsgUsers, _ := s.GetSetting(ctx, "antimsg_users:"+chatStr)
		if rawAntiMsgUsers != "" {
			targetUsers := strings.Split(rawAntiMsgUsers, ",")
			senderStr := sender.String()
			for _, uStr := range targetUsers {
				uStr = strings.TrimSpace(uStr)
				if uStr == "" {
					continue
				}
				uJID, err := types.ParseJID(uStr)
				if err != nil {
					continue
				}
				if waSender.IsSameUserRaw(ctx, client, uJID, evt.Info.Sender) {
					slog.Debug("antimsg: deleting message from targeted participant", "chat", chatStr, "sender", senderStr)
					_, _ = client.SendMessage(ctx, evt.Info.Chat, client.BuildRevoke(evt.Info.Chat, evt.Info.Sender, evt.Info.ID))
					return true
				}
			}
		}
	}

	// Check AntiSpam rate limits
	rawAntiSpamStatus, _ := s.GetSetting(ctx, "antispam_status:"+chatStr)
	if rawAntiSpamStatus == "on" {
		info, err := client.GetGroupInfo(ctx, evt.Info.Chat)
		if err == nil && !waSender.IsAdminRaw(ctx, client, info, sender) {
			rawMax, _ := s.GetSetting(ctx, "antispam_max:"+chatStr)
			maxMsgs, _ := strconv.Atoi(rawMax)
			if maxMsgs <= 0 {
				maxMsgs = 5
			}
			if checkSpamLimit(chatStr, sender.String(), maxMsgs) {
				action, _ := s.GetSetting(ctx, "antispam_action:"+chatStr)
				if action == "" {
					action = "delete"
				}
				slog.Debug("antispam: message rate limit exceeded", "chat", chatStr, "sender", sender.String(), "action", action)
				botIsAdmin := false
				if client.Store.ID != nil {
					botIsAdmin = waSender.IsAdminRaw(ctx, client, info, *client.Store.ID)
				}
				if botIsAdmin {
					_, _ = client.SendMessage(ctx, evt.Info.Chat, client.BuildRevoke(evt.Info.Chat, evt.Info.Sender, evt.Info.ID))
					if action == "kick" {
						_, _ = client.UpdateGroupParticipants(ctx, evt.Info.Chat, []types.JID{evt.Info.Sender}, whatsmeow.ParticipantChangeRemove)
					}
					resolvedJID, username := waSender.ResolveMentionRaw(ctx, client, evt.Info.Sender)
					textMsg := fmt.Sprintf("AntiSpam: @%s message rate limit exceeded (action: %s).", username, action)
					_, _ = client.SendMessage(ctx, evt.Info.Chat, &waE2E.Message{
						ExtendedTextMessage: &waE2E.ExtendedTextMessage{
							Text: &textMsg,
							ContextInfo: &waE2E.ContextInfo{
								MentionedJID: []string{resolvedJID.String()},
							},
						},
					})
					return true
				}
			}
		}
	}

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

	// Check if sender is admin or sudo user (exempt from AntiLink/AntiWord)
	info, err := client.GetGroupInfo(ctx, evt.Info.Chat)
	if err != nil {
		return false
	}

	if waSender.IsAdminRaw(ctx, client, info, sender) || waSender.IsSudoRaw(ctx, client, sender) {
		return false
	}

	violation := false
	reason := ""
	violationType := "" // "antilink" or "antiword"

	if antiLinkEnabled {
		lowerText := strings.ToLower(text)
		mode, _ := s.GetSetting(ctx, "antilink_mode:"+chatStr)
		if mode == "custom" {
			customStr, _ := s.GetSetting(ctx, "antilink_custom:"+chatStr)
			if customStr == "" {
				customStr = "chat.whatsapp.com"
			}
			domains := strings.Split(customStr, ",")
			for _, d := range domains {
				d = strings.TrimSpace(strings.ToLower(d))
				if d != "" && strings.Contains(lowerText, d) {
					violation = true
					reason = fmt.Sprintf("banned link (%s)", d)
					violationType = "antilink"
					break
				}
			}
		} else {
			if strings.Contains(lowerText, "http://") || strings.Contains(lowerText, "https://") || strings.Contains(lowerText, "www.") || strings.Contains(lowerText, ".com") || strings.Contains(lowerText, ".net") || strings.Contains(lowerText, ".org") {
				violation = true
				reason = "links"
				violationType = "antilink"
			}
		}
	}

	if !violation && len(bannedWords) > 0 {
		lowerText := strings.ToLower(text)
		for _, w := range bannedWords {
			if strings.Contains(lowerText, w) {
				violation = true
				reason = fmt.Sprintf("banned word (%s)", w)
				violationType = "antiword"
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
			// Delete violating message
			_, _ = client.SendMessage(ctx, evt.Info.Chat, client.BuildRevoke(evt.Info.Chat, evt.Info.Sender, evt.Info.ID))
			resolvedJID, username := waSender.ResolveMentionRaw(ctx, client, evt.Info.Sender)

			actionKey := violationType + "_action:" + chatStr
			action, _ := s.GetSetting(ctx, actionKey)
			action = strings.ToLower(strings.TrimSpace(action))

			switch action {
			case "kick":
				_, _ = client.UpdateGroupParticipants(ctx, evt.Info.Chat, []types.JID{evt.Info.Sender}, whatsmeow.ParticipantChangeRemove)
				textMsg := fmt.Sprintf("Message from @%s deleted and participant kicked: contains %s.", username, reason)
				_, _ = client.SendMessage(ctx, evt.Info.Chat, &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &textMsg,
						ContextInfo: &waE2E.ContextInfo{
							MentionedJID: []string{resolvedJID.String()},
						},
					},
				})

			case "warn":
				maxWarnKey := violationType + "_maxwarn:" + chatStr
				maxWarnStr, _ := s.GetSetting(ctx, maxWarnKey)
				maxWarn := 3
				if parsed, err := strconv.Atoi(maxWarnStr); err == nil && parsed > 0 {
					maxWarn = parsed
				}

				warnsKey := violationType + "_warns:" + chatStr + ":" + evt.Info.Sender.ToNonAD().String()
				currWarnStr, _ := s.GetSetting(ctx, warnsKey)
				currWarns := 0
				if parsed, err := strconv.Atoi(currWarnStr); err == nil {
					currWarns = parsed
				}
				currWarns++

				if currWarns >= maxWarn {
					_, _ = client.UpdateGroupParticipants(ctx, evt.Info.Chat, []types.JID{evt.Info.Sender}, whatsmeow.ParticipantChangeRemove)
					_ = s.PutSetting(ctx, warnsKey, "0")
					textMsg := fmt.Sprintf("⚠️ @%s reached maximum warnings (%d/%d) for %s! Message deleted and participant kicked.", username, currWarns, maxWarn, reason)
					_, _ = client.SendMessage(ctx, evt.Info.Chat, &waE2E.Message{
						ExtendedTextMessage: &waE2E.ExtendedTextMessage{
							Text: &textMsg,
							ContextInfo: &waE2E.ContextInfo{
								MentionedJID: []string{resolvedJID.String()},
							},
						},
					})
				} else {
					_ = s.PutSetting(ctx, warnsKey, strconv.Itoa(currWarns))
					textMsg := fmt.Sprintf("⚠️ Warning for @%s (%d/%d): Message deleted for %s. Reaching %d warnings will result in a kick!", username, currWarns, maxWarn, reason, maxWarn)
					_, _ = client.SendMessage(ctx, evt.Info.Chat, &waE2E.Message{
						ExtendedTextMessage: &waE2E.ExtendedTextMessage{
							Text: &textMsg,
							ContextInfo: &waE2E.ContextInfo{
								MentionedJID: []string{resolvedJID.String()},
							},
						},
					})
				}

			default: // "delete"
				textMsg := fmt.Sprintf("Message from @%s deleted: contains %s.", username, reason)
				_, _ = client.SendMessage(ctx, evt.Info.Chat, &waE2E.Message{
					ExtendedTextMessage: &waE2E.ExtendedTextMessage{
						Text: &textMsg,
						ContextInfo: &waE2E.ContextInfo{
							MentionedJID: []string{resolvedJID.String()},
						},
					},
				})
			}
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

	lowerText := strings.ToLower(text)
	botName := GetBotName(context.Background(), client)
	lowerBotName := strings.ToLower(botName)

	// 0. Check if text contains custom botName, "whatsrook", or "rook" as keywords
	if (lowerBotName != "" && strings.Contains(lowerText, lowerBotName)) || strings.Contains(lowerText, "whatsrook") || strings.Contains(lowerText, "rook") {
		return true
	}

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
