// Group management commands – invite, kick, promote, demote, tag all, etc.
package commands

import (
	"fmt"
	"slices"
	"strings"

	"whatsrook/store/sqlstore"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
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
		Description: "Create a poll. Usage: poll Question | Option 1 | Option 2 | ...",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handlePoll,
	})
	Register(&Command{
		Name:        "lockpoll",
		Description: "Create a single-choice poll. Usage: lockpoll Question | Option 1 | Option 2 | ...",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleLockPoll,
	})
	Register(&Command{
		Name:        "invite",
		Description: "Get the group invite link",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleInvite,
	})
}

func handleTagAll(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group.")
	}
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can tag everyone.")
	}

	var sb strings.Builder
	sb.WriteString(" *Everyone*\n")
	if ctx.RawArgs != "" {
		sb.WriteString(fmt.Sprintf("Message: *%s*\n\n", ctx.RawArgs))
	} else {
		sb.WriteString("\n")
	}

	var mentions []types.JID
	for _, p := range info.Participants {
		resolvedJID, username := ctx.ResolveMention(p.JID)
		sb.WriteString(fmt.Sprintf("@%s ", username))
		mentions = append(mentions, resolvedJID)
	}

	return ctx.ReplyWithMentions(sb.String(), mentions)
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
		return ctx.Reply("Please reply to a member, tag them, or type their phone number to kick.")
	}

	var kicked []string
	var kickedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeRemove)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.Reply(fmt.Sprintf(" Failed to kick %s: %v", username, err))
		} else {
			kicked = append(kicked, "@"+username)
			kickedJIDs = append(kickedJIDs, resolvedJID)
		}
	}

	if len(kicked) > 0 {
		return ctx.ReplyWithMentions(fmt.Sprintf(" Kicked: %s", strings.Join(kicked, ", ")), kickedJIDs)
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
		return ctx.Reply("Please type a phone number to add.")
	}

	var added []string
	var addedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeAdd)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.Reply(fmt.Sprintf(" Failed to add %s: %v", username, err))
		} else {
			added = append(added, "@"+username)
			addedJIDs = append(addedJIDs, resolvedJID)
		}
	}

	if len(added) > 0 {
		return ctx.ReplyWithMentions(fmt.Sprintf(" Added: %s", strings.Join(added, ", ")), addedJIDs)
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
		return ctx.Reply("Please reply to a member, tag them, or type their phone number to promote.")
	}

	var promoted []string
	var promotedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangePromote)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.Reply(fmt.Sprintf(" Failed to promote %s: %v", username, err))
		} else {
			promoted = append(promoted, "@"+username)
			promotedJIDs = append(promotedJIDs, resolvedJID)
		}
	}

	if len(promoted) > 0 {
		return ctx.ReplyWithMentions(fmt.Sprintf(" Promoted: %s", strings.Join(promoted, ", ")), promotedJIDs)
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
		return ctx.Reply("Please reply to a member, tag them, or type their phone number to demote.")
	}

	var demoted []string
	var demotedJIDs []types.JID
	for _, target := range targets {
		_, err := ctx.Client.UpdateGroupParticipants(ctx.Ctx, ctx.Chat, []types.JID{target}, whatsmeow.ParticipantChangeDemote)
		resolvedJID, username := ctx.ResolveMention(target)
		if err != nil {
			_ = ctx.Reply(fmt.Sprintf(" Failed to demote %s: %v", username, err))
		} else {
			demoted = append(demoted, "@"+username)
			demotedJIDs = append(demotedJIDs, resolvedJID)
		}
	}

	if len(demoted) > 0 {
		return ctx.ReplyWithMentions(fmt.Sprintf(" Demoted: %s", strings.Join(demoted, ", ")), demotedJIDs)
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
		return ctx.Reply("Usage: group <open|close|lock|unlock>")
	}

	action := strings.ToLower(ctx.Args[0])
	switch action {
	case "open":
		err = ctx.Client.SetGroupAnnounce(ctx.Ctx, ctx.Chat, false)
		if err != nil {
			return ctx.Reply(fmt.Sprintf(" Failed to open group: %v", err))
		}
		return ctx.Reply("Group opened. Everyone can send messages.")
	case "close":
		err = ctx.Client.SetGroupAnnounce(ctx.Ctx, ctx.Chat, true)
		if err != nil {
			return ctx.Reply(fmt.Sprintf(" Failed to close group: %v", err))
		}
		return ctx.Reply("Group closed. Only admins can send messages.")
	case "lock":
		err = ctx.Client.SetGroupLocked(ctx.Ctx, ctx.Chat, true)
		if err != nil {
			return ctx.Reply(fmt.Sprintf(" Failed to lock group: %v", err))
		}
		return ctx.Reply("Group locked. Only admins can edit group settings.")
	case "unlock":
		err = ctx.Client.SetGroupLocked(ctx.Ctx, ctx.Chat, false)
		if err != nil {
			return ctx.Reply(fmt.Sprintf(" Failed to unlock group: %v", err))
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
		return ctx.Reply("Usage: antilink [on/off]")
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
		return err
	}

	return ctx.Reply(fmt.Sprintf(" Anti-link protection turned %s.", state))
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
		return ctx.Reply("Usage:\n- antiword add [word]\n- antiword del [word]\n- antiword list")
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
			return ctx.Reply(fmt.Sprintf("ℹ Word %q is already banned.", wordToAdd))
		}
		words = append(words, wordToAdd)
		err = s.PutSetting(ctx.Ctx, settingKey, strings.Join(words, " "))
		if err != nil {
			return err
		}
		return ctx.Reply(fmt.Sprintf(" Banned word %q added.", wordToAdd))

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
			return ctx.Reply(fmt.Sprintf("ℹ Word %q was not banned.", wordToDel))
		}
		err = s.PutSetting(ctx.Ctx, settingKey, strings.Join(newWords, " "))
		if err != nil {
			return err
		}
		return ctx.Reply(fmt.Sprintf(" Banned word %q removed.", wordToDel))

	case "list":
		if len(words) == 0 {
			return ctx.Reply("ℹ No banned words configured in this group.")
		}
		return ctx.Reply(fmt.Sprintf(" *Banned Words list:*\n- %s", strings.Join(words, "\n- ")))

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
	parts := strings.Split(ctx.RawArgs, "|")
	if len(parts) < 3 {
		return ctx.Reply("Usage: poll Question | Option 1 | Option 2 | ...")
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

	pollMsg := ctx.Client.BuildPollCreation(question, options, 0)
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, pollMsg)
	return err
}

func handleLockPoll(ctx *Context) error {
	parts := strings.Split(ctx.RawArgs, "|")
	if len(parts) < 3 {
		return ctx.Reply("Usage: lockpoll Question | Option 1 | Option 2 | ...")
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

	pollMsg := ctx.Client.BuildPollCreation(question, options, 1)
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, pollMsg)
	return err
}

func handleInvite(ctx *Context) error {
	info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get group info: %v", err))
	}
	if !ctx.IsSenderAdmin(info) {
		return ctx.Reply("Only group admins can retrieve the invite link.")
	}

	link, err := ctx.Client.GetGroupInviteLink(ctx.Ctx, ctx.Chat, false)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get invite link: %v", err))
	}
	return ctx.Reply(link)
}
