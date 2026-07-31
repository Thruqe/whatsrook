// Group management commands – invite, kick, promote, demote, tag all, etc.
package commands

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"whatsrook/store/sqlstore"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func init() {
	Register(&Command{
		Name:        "tagall",
		Aliases:     []string{"everyone"},
		Description: "Mention everyone in the group",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleTagAll,
	})
	Register(&Command{
		Name:        "kick",
		Description: "Remove a member from the group (reply, tag, or number)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleKick,
	})
	Register(&Command{
		Name:        "add",
		Description: "Add a member to the group (phone number/JID)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAdd,
	})
	Register(&Command{
		Name:        "promote",
		Description: "Promote a member to admin (reply, tag, or number)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handlePromote,
	})
	Register(&Command{
		Name:        "demote",
		Description: "Demote a member from admin (reply, tag, or number)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleDemote,
	})
	Register(&Command{
		Name:        "group",
		Description: "Manage group settings (open, close, lock, unlock)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleGroup,
	})
	Register(&Command{
		Name:        "antilink",
		Description: "Enable or disable anti-link protection (on/off)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAntiLink,
	})
	Register(&Command{
		Name:        "antiword",
		Description: "Manage banned words (add [word], del [word], list)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleAntiWord,
	})
	Register(&Command{
		Name:        "gstats",
		Description: "Provide statistics on the most active group participants",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleGStats,
	})
	Register(&Command{
		Name:        "poll",
		Aliases:     []string{"lockpoll"},
		Description: "Create a poll with single or multiple choice selection buttons",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handlePoll,
	})
	Register(&Command{
		Name:        "invite",
		Description: "Get the group invite link",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleInvite,
	})
	Register(&Command{
		Name:        "listonline",
		Aliases:     []string{"online", "onlines", "list-online"},
		Description: "List online participants in the current group",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleListOnline,
	})
}

func handleTagAll(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can tag everyone.")
	}

	var sb strings.Builder
	sb.WriteString("@all")
	if ctx.RawArgs != "" {
		sb.WriteString("\nMessage: *")
		sb.WriteString(ctx.RawArgs)
		sb.WriteString("*")
	}

	var participantJIDs []types.JID
	for _, p := range info.Participants {
		if !p.JID.IsEmpty() {
			participantJIDs = append(participantJIDs, p.JID)
		}
	}

	groupSubject := info.GroupName.Name
	if groupSubject == "" {
		groupSubject = "Group"
	}

	return ctx.ReplyWithGroupMention(sb.String(), ctx.Chat, groupSubject, participantJIDs)
}

func handleKick(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can kick members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to kick members.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %skick @user\n- %skick 1234567890\n- Reply to a user's message with %skick", p, p, p))
	}

	var kicked []string
	var kickedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeRemove)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.ReplyWithMentions(fmt.Sprintf("Failed to kick @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			kicked = append(kicked, "@"+username)
			kickedJIDs = append(kickedJIDs, resolvedJID)
		}
	}

	if len(kicked) > 0 {
		return ctx.ReplyWithMentions(fmt.Sprintf("Kicked: %s", strings.Join(kicked, ", ")), kickedJIDs)
	}
	return nil
}

func handleAdd(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can add members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to add members.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sadd 1234567890\n- %sadd 1234567890 9876543210", p, p))
	}

	var added []string
	var addedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeAdd)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.ReplyWithMentions(fmt.Sprintf("Failed to add @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			added = append(added, "@"+username)
			addedJIDs = append(addedJIDs, resolvedJID)
		}
	}

	if len(added) > 0 {
		return ctx.ReplyWithMentions(fmt.Sprintf("Added: %s", strings.Join(added, ", ")), addedJIDs)
	}
	return nil
}

func handlePromote(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can promote members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to promote members.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %spromote @user\n- %spromote 1234567890\n- Reply to a user's message with %spromote", p, p, p))
	}

	var promoted []string
	var promotedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangePromote)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.ReplyWithMentions(fmt.Sprintf("Failed to promote @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			promoted = append(promoted, "@"+username)
			promotedJIDs = append(promotedJIDs, resolvedJID)
		}
	}

	if len(promoted) > 0 {
		return ctx.ReplyWithMentions(fmt.Sprintf("Promoted: %s", strings.Join(promoted, ", ")), promotedJIDs)
	}
	return nil
}

func handleDemote(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can demote members.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to demote members.")
	}

	targets := ctx.GetTargets()
	if len(targets) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sdemote @user\n- %sdemote 1234567890\n- Reply to a user's message with %sdemote", p, p, p))
	}

	var demoted []string
	var demotedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeDemote)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.ReplyWithMentions(fmt.Sprintf("Failed to demote @%s: %v", username, err), []types.JID{resolvedJID})
		} else {
			demoted = append(demoted, "@"+username)
			demotedJIDs = append(demotedJIDs, resolvedJID)
		}
	}

	if len(demoted) > 0 {
		return ctx.ReplyWithMentions(fmt.Sprintf("Demoted: %s", strings.Join(demoted, ", ")), demotedJIDs)
	}
	return nil
}

func handleGroup(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can change group settings.")
	}
	if !ctx.AmIAdmin(info) {
		return ctx.Reply("The bot must be an admin to change group settings.")
	}

	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sgroup open\n- %sgroup close\n- %sgroup lock\n- %sgroup unlock", p, p, p, p))
	}

	action := strings.ToLower(ctx.Args[0])
	switch action {
	case "open":
		err = ctx.Client.SetGroupAnnounce(ctx.Ctx, ctx.Chat, false)
		if err != nil {
			return ctx.Reply(fmt.Sprintf("Failed to open group: %v", err))
		}
		return ctx.Reply("Group opened. Everyone can send messages.")
	case "close":
		err = ctx.Client.SetGroupAnnounce(ctx.Ctx, ctx.Chat, true)
		if err != nil {
			return ctx.Reply(fmt.Sprintf("Failed to close group: %v", err))
		}
		return ctx.Reply("Group closed. Only admins can send messages.")
	case "lock":
		err = ctx.Client.SetGroupLocked(ctx.Ctx, ctx.Chat, true)
		if err != nil {
			return ctx.Reply(fmt.Sprintf("Failed to lock group: %v", err))
		}
		return ctx.Reply("Group locked. Only admins can edit group settings.")
	case "unlock":
		err = ctx.Client.SetGroupLocked(ctx.Ctx, ctx.Chat, false)
		if err != nil {
			return ctx.Reply(fmt.Sprintf("Failed to unlock group: %v", err))
		}
		return ctx.Reply("Group unlocked. Everyone can edit group settings.")
	default:
		return ctx.Reply("Invalid action. Usage: group <open|close|lock|unlock>")
	}
}

func handleAntiLink(ctx *Context) error {
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can change anti-link settings.")
	}

	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %santilink on\n- %santilink off", p, p))
	}

	state := strings.ToLower(ctx.Args[0])
	if state != "on" && state != "off" {
		return ctx.Reply("Invalid state. Usage: antilink [on/off]")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	err = s.PutSetting(ctx.Ctx, "antilink:"+ctx.Chat.String(), state)
	if err != nil {
		return ctx.Reply("Failed to save anti-link setting.")
	}

	return ctx.Reply(fmt.Sprintf("Anti-link protection turned %s.", state))
}

func handleAntiWord(ctx *Context) error {
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can change anti-word settings.")
	}

	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %santiword add <word>\n- %santiword del <word>\n- %santiword list\nExamples:\n- %santiword add spamword", p, p, p, p))
	}

	sub := strings.ToLower(ctx.Args[0])
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	settingKey := "antiword:" + ctx.Chat.String()
	raw, _ := s.GetSetting(ctx.Ctx, settingKey)
	words := strings.Fields(raw)

	switch sub {
	case "add":
		if len(ctx.Args) < 2 {
			return ctx.Reply("Please specify the word to add.")
		}
		wordToAdd := strings.ToLower(ctx.Args[1])
		exists := slices.Contains(words, wordToAdd)
		if exists {
			return ctx.Reply(fmt.Sprintf("Word %q is already banned.", wordToAdd))
		}
		words = append(words, wordToAdd)
		err = s.PutSetting(ctx.Ctx, settingKey, strings.Join(words, " "))
		if err != nil {
			return ctx.Reply("Failed to save anti-word setting.")
		}
		return ctx.Reply(fmt.Sprintf("Banned word %q added.", wordToAdd))

	case "del", "remove":
		if len(ctx.Args) < 2 {
			return ctx.Reply("Please specify the word to remove.")
		}
		wordToDel := strings.ToLower(ctx.Args[1])
		found := false
		newWords := []string{}
		for _, w := range words {
			if w == wordToDel {
				found = true
			} else {
				newWords = append(newWords, w)
			}
		}
		if !found {
			return ctx.Reply(fmt.Sprintf("Word %q was not banned.", wordToDel))
		}
		err = s.PutSetting(ctx.Ctx, settingKey, strings.Join(newWords, " "))
		if err != nil {
			return ctx.Reply("Failed to save anti-word setting.")
		}
		return ctx.Reply(fmt.Sprintf("Banned word %q removed.", wordToDel))

	case "list":
		if len(words) == 0 {
			return ctx.Reply("No banned words configured in this group.")
		}
		return ctx.Reply(fmt.Sprintf("Banned Words list:\n- %s", strings.Join(words, "\n- ")))

	default:
		return ctx.Reply("Invalid action. Usage: antiword <add|del|list>")
	}
}

func handleGStats(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}
	db := s.GetDB()
	if db == nil {
		return ctx.Reply("Database unavailable.")
	}

	chatStr := ctx.Chat.String()

	var totalMsgs int
	err := db.QueryRow(ctx.Ctx, `SELECT COUNT(*) FROM message_secrets WHERE chat_jid=$1`, chatStr).Scan(&totalMsgs)
	if err != nil {
		return err
	}

	if totalMsgs == 0 {
		return ctx.Reply("No message activity found in database for this group.")
	}

	var activeUsers int
	err = db.QueryRow(ctx.Ctx, `SELECT COUNT(DISTINCT sender_jid) FROM message_secrets WHERE chat_jid=$1`, chatStr).Scan(&activeUsers)
	if err != nil {
		activeUsers = 0
	}

	rows, err := db.Query(ctx.Ctx, `
		SELECT sender_jid, COUNT(*) as total 
		FROM message_secrets 
		WHERE chat_jid=$1 
		GROUP BY sender_jid 
		ORDER BY total DESC 
		LIMIT 10
	`, chatStr)
	if err != nil {
		return err
	}
	defer rows.Close()

	var mentions []types.JID
	var sb strings.Builder
	sb.WriteString(" *Group Activity Statistics (from message secrets)*\n\n")
	sb.WriteString(fmt.Sprintf("• Total messages tracked: %d\n", totalMsgs))
	sb.WriteString(fmt.Sprintf("• Unique active senders: %d\n\n", activeUsers))
	sb.WriteString(" *Top Active Participants:*\n")

	rank := 1
	for rows.Next() {
		var userStr string
		var count int
		if err := rows.Scan(&userStr, &count); err == nil {
			uj, err := types.ParseJID(userStr)
			if err == nil {
				uj = uj.ToNonAD()
				resolvedJID, username := ctx.ResolveMention(uj)
				fmt.Fprintf(&sb, "%d. @%s (%d msgs)\n", rank, username, count)
				mentions = append(mentions, resolvedJID)
				rank++
			}
		}
	}

	return ctx.ReplyWithMentions(sb.String(), mentions)
}

func handlePoll(ctx *Context) error {
	raw := strings.TrimSpace(ctx.RawArgs)
	if raw == "" {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: %spoll Question | Option 1 | Option 2 | ...", p))
	}

	selectableCount := -1
	if strings.HasPrefix(raw, "--single ") || strings.HasPrefix(raw, "-s ") || strings.HasPrefix(raw, "single ") {
		selectableCount = 1
		raw = strings.TrimSpace(raw[strings.Index(raw, " "):])
	} else if strings.HasPrefix(raw, "--multi ") || strings.HasPrefix(raw, "-m ") || strings.HasPrefix(raw, "multi ") || strings.HasPrefix(raw, "multiple ") {
		selectableCount = 0
		raw = strings.TrimSpace(raw[strings.Index(raw, " "):])
	}

	parts := strings.Split(raw, "|")
	if len(parts) < 3 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: %spoll Question | Option 1 | Option 2 | ...", p))
	}

	question := strings.TrimSpace(parts[0])
	var options []string
	for _, opt := range parts[1:] {
		trimmed := strings.TrimSpace(opt)
		if trimmed != "" {
			options = append(options, trimmed)
		}
	}
	if len(options) < 2 {
		return ctx.Reply("Please provide at least 2 options.")
	}

	if selectableCount >= 0 {
		pollMsg := ctx.Client.BuildPollCreation(question, options, selectableCount)
		_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, pollMsg)
		return err
	}

	var sb strings.Builder
	sb.WriteString("Poll Creation\n\nQuestion: ")
	sb.WriteString(question)
	sb.WriteString("\nOptions:\n")
	for i, opt := range options {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, opt)
	}
	sb.WriteString("\nSelect poll type below to create poll.")

	p := ctx.GetPrefix()
	pollArgs := question + " | " + strings.Join(options, " | ")
	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: new(sb.String()),
					FooterText:  new(fmt.Sprintf("%s Interactive Poll", ctx.GetBotName())),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons: []*waE2E.ButtonsMessage_Button{
						{
							ButtonID: new(p + "poll --single " + pollArgs),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("SINGLE CHOICE"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(p + "poll --multi " + pollArgs),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("MULTIPLE CHOICE"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
					},
				},
			},
		},
	}

	bizNode := waBinary.Node{
		Tag:   "biz",
		Attrs: waBinary.Attrs{},
		Content: []waBinary.Node{
			{
				Tag: "interactive",
				Attrs: waBinary.Attrs{
					"type": "native_flow",
					"v":    "1",
				},
				Content: []waBinary.Node{
					{
						Tag: "native_flow",
						Attrs: waBinary.Attrs{
							"v":    "9",
							"name": "mixed",
						},
					},
				},
			},
		},
	}

	extra := whatsmeow.SendRequestExtra{
		AdditionalNodes: &[]waBinary.Node{bizNode},
	}

	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	return err
}

func handleInvite(ctx *Context) error {
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can retrieve the invite link.")
	}

	link, err := ctx.Client.GetGroupInviteLink(ctx.Ctx, ctx.Chat, false)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to get invite link: %v", err))
	}
	return ctx.Reply(link)
}

var (
	presenceMu  sync.RWMutex
	presenceMap = make(map[string]PresenceInfo)
)

type PresenceInfo struct {
	LastSeen time.Time
	IsOnline bool
}

func TrackPresence(jid types.JID, isOnline bool) {
	if jid.IsEmpty() {
		return
	}
	key := jid.ToNonAD().String()
	presenceMu.Lock()
	presenceMap[key] = PresenceInfo{
		LastSeen: time.Now(),
		IsOnline: isOnline,
	}
	presenceMu.Unlock()
	slog.Debug("TrackPresence update", "jid", key, "isOnline", isOnline)
}

func IsUserOnline(jid types.JID, client *whatsmeow.Client) bool {
	if jid.IsEmpty() {
		return false
	}
	targetKey := jid.ToNonAD().String()

	presenceMu.RLock()
	info, exists := presenceMap[targetKey]
	presenceMu.RUnlock()

	if exists && (info.IsOnline || time.Since(info.LastSeen) < 15*time.Minute) {
		slog.Debug("IsUserOnline check: direct match online", "jid", targetKey, "lastSeen", info.LastSeen)
		return true
	}

	if client != nil && client.Store != nil && client.Store.LIDs != nil {
		ctx := context.Background()
		if jid.Server == types.HiddenUserServer {
			pn, err := client.Store.LIDs.GetPNForLID(ctx, jid)
			if err == nil && !pn.IsEmpty() {
				pnKey := pn.ToNonAD().String()
				presenceMu.RLock()
				pnInfo, pnExists := presenceMap[pnKey]
				presenceMu.RUnlock()
				if pnExists && (pnInfo.IsOnline || time.Since(pnInfo.LastSeen) < 15*time.Minute) {
					slog.Debug("IsUserOnline check: PN match online for LID", "lid", targetKey, "pn", pnKey)
					return true
				}
			}
		} else {
			lid, err := client.Store.LIDs.GetLIDForPN(ctx, jid)
			if err == nil && !lid.IsEmpty() {
				lidKey := lid.ToNonAD().String()
				presenceMu.RLock()
				lidInfo, lidExists := presenceMap[lidKey]
				presenceMu.RUnlock()
				if lidExists && (lidInfo.IsOnline || time.Since(lidInfo.LastSeen) < 15*time.Minute) {
					slog.Debug("IsUserOnline check: LID match online for PN", "pn", targetKey, "lid", lidKey)
					return true
				}
			}
		}
	}

	slog.Debug("IsUserOnline check: offline or unknown", "jid", targetKey)
	return false
}

func handleListOnline(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		slog.Debug("handleListOnline: not a group chat", "chat", ctx.Chat.String())
		return ctx.Reply("This command can only be used in a group.")
	}

	slog.Debug("handleListOnline executing", "group", ctx.Chat.String())
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		slog.Error("handleListOnline: failed to get group info", "group", ctx.Chat.String(), "err", err)
		return ctx.Reply(fmt.Sprintf("Failed to get group info: %v", err))
	}

	total := len(info.Participants)
	slog.Debug("handleListOnline retrieved group info", "group", ctx.Chat.String(), "participant_count", total)

	if total == 0 {
		return ctx.Reply("No participants found in this group.")
	}

	// 1. Send status message to prompt WhatsApp servers to trigger group-wide delivery receipts
	_ = ctx.Reply("Fetching online participants...")

	// 2. Query SQLite database for stored receipt/presence activity for this group
	if s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore); ok {
		dbActive, err := s.GetActiveGroupParticipants(ctx.Ctx, ctx.Chat, 24*time.Hour)
		if err == nil && len(dbActive) > 0 {
			slog.Debug("handleListOnline: loaded stored active participants from SQLite", "count", len(dbActive))
			for _, userJID := range dbActive {
				TrackPresence(userJID, true)
			}
		}
	}

	// Build set of expected participant JID keys (LID & PN formats)
	expectedJIDs := make(map[string]types.JID)
	var mu sync.Mutex
	receivedCount := 0
	doneChan := make(chan struct{})

	for _, p := range info.Participants {
		nonAD := p.JID.ToNonAD()
		expectedJIDs[nonAD.String()] = nonAD
		if ctx.Client.Store != nil && ctx.Client.Store.LIDs != nil {
			if nonAD.Server == types.HiddenUserServer {
				if pn, err := ctx.Client.Store.LIDs.GetPNForLID(ctx.Ctx, nonAD); err == nil && !pn.IsEmpty() {
					expectedJIDs[pn.ToNonAD().String()] = nonAD
				}
			} else {
				if lid, err := ctx.Client.Store.LIDs.GetLIDForPN(ctx.Ctx, nonAD); err == nil && !lid.IsEmpty() {
					expectedJIDs[lid.ToNonAD().String()] = nonAD
				}
			}
		}
	}

	// Register temporary event listener for WhatsApp presence and receipt response stanzas
	handlerID := ctx.Client.AddEventHandler(func(evt any) {
		switch pEvt := evt.(type) {
		case *events.Presence:
			fromKey := pEvt.From.ToNonAD().String()
			mu.Lock()
			if targetJID, isExpected := expectedJIDs[fromKey]; isExpected {
				slog.Debug("handleListOnline: received presence stanza from WhatsApp", "from", fromKey, "unavailable", pEvt.Unavailable)
				TrackPresence(targetJID, !pEvt.Unavailable)
				delete(expectedJIDs, fromKey)
				receivedCount++
				if len(expectedJIDs) == 0 {
					select {
					case <-doneChan:
					default:
						close(doneChan)
					}
				}
			}
			mu.Unlock()

		case *events.Receipt:
			senderKey := pEvt.Sender.ToNonAD().String()
			if !pEvt.Sender.IsEmpty() {
				mu.Lock()
				if targetJID, isExpected := expectedJIDs[senderKey]; isExpected {
					slog.Debug("handleListOnline: received delivery receipt from WhatsApp", "sender", senderKey)
					TrackPresence(targetJID, true)
					delete(expectedJIDs, senderKey)
					receivedCount++
					if len(expectedJIDs) == 0 {
						select {
						case <-doneChan:
						default:
							close(doneChan)
						}
					}
				}
				mu.Unlock()
			}
		}
	})
	defer ctx.Client.RemoveEventHandler(handlerID)

	// Dispatch SubscribePresence to WhatsApp for all group participants
	for _, p := range info.Participants {
		_ = ctx.Client.SubscribePresence(ctx.Ctx, p.JID)
	}

	// Check if presenceMap already has cached online presence for group members
	cachedOnlineCount := 0
	for _, p := range info.Participants {
		if IsUserOnline(p.JID, ctx.Client) {
			cachedOnlineCount++
		}
	}

	// If cached online records are small, wait up to 2s for live presence/receipt stanzas
	if cachedOnlineCount < 2 {
		select {
		case <-doneChan:
			slog.Debug("handleListOnline: presence/receipt stanzas collected", "count", receivedCount)
		case <-time.After(2000 * time.Millisecond):
			slog.Debug("handleListOnline: presence wait window ended", "received", receivedCount, "total", total)
		}
	}

	// Collect online participants
	var onlineJIDs []types.JID
	var displayNames []string

	for _, p := range info.Participants {
		if IsUserOnline(p.JID, ctx.Client) {
			resolvedJID, username := ctx.ResolveMention(p.JID)
			onlineJIDs = append(onlineJIDs, resolvedJID)
			displayNames = append(displayNames, "@"+username)
			slog.Debug("handleListOnline: participant online", "participant", p.JID.String(), "username", username)
		} else {
			slog.Debug("handleListOnline: participant offline", "participant", p.JID.String())
		}
	}

	slog.Debug("handleListOnline complete", "group", ctx.Chat.String(), "total_participants", total, "online_count", len(onlineJIDs))

	if len(onlineJIDs) == 0 {
		return ctx.Reply("No online participants detected in this group.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Online Participants (%d):\n\n", len(onlineJIDs)))
	for _, name := range displayNames {
		fmt.Fprintf(&sb, "- %s\n", name)
	}

	return ctx.ReplyWithMentions(sb.String(), onlineJIDs)
}
