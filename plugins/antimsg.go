// AntiMsg command – automatically delete incoming messages from specified group participants.
package plugins

import (
	"fmt"
	"strings"

	"whatsrook/store/sqlstore"

	"go.mau.fi/whatsmeow/types"
)

func init() {
	Register(&Command{
		Name:        "antimsg",
		Aliases:     []string{"anti-msg", "antimessage"},
		Description: "Automatically delete messages sent by specified group participants",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleAntiMsg,
	})
}

func handleAntiMsg(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	chatKey := ctx.Chat.String()
	statusKey := "antimsg_status:" + chatKey
	usersKey := "antimsg_users:" + chatKey

	args := strings.Fields(ctx.RawArgs)
	sub := ""
	if len(args) > 0 {
		sub = strings.ToLower(args[0])
	}

	// Extract target participants if mentioned or quoted
	targets := extractTargetParticipants(ctx, args)

	// If targets are provided (via reply, @mention, or argument), default action is to ADD them and activate AntiMsg
	if len(targets) > 0 && sub != "del" && sub != "remove" && sub != "delete" {
		rawUsers, _ := s.GetSetting(ctx.Ctx, usersKey)
		users := splitCSV(rawUsers)

		var addedMentions []types.JID
		var addedUsernames []string

		for _, t := range targets {
			tStr := t.ToNonAD().String()
			if !containsString(users, tStr) {
				users = append(users, tStr)
			}
			resolvedJID, username := ctx.ResolveMention(t)
			addedMentions = append(addedMentions, resolvedJID)
			addedUsernames = append(addedUsernames, "@"+username)
		}

		_ = s.PutSetting(ctx.Ctx, usersKey, strings.Join(users, ","))
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")

		p := ctx.GetPrefix()
		bodyText := fmt.Sprintf("╭━━━〔 ANTIMSG ACTIVATED 〕━━━\n│ Status : ON\n│ Added  : %s\n│ Total  : %d targeted user(s)\n╰━━━━━━━━━━━━━━━━━━━━━━\n\nAntiMsg is active! Messages from targeted participants will be automatically deleted.", strings.Join(addedUsernames, ", "), len(users))

		buttons := []struct{ ID, Text string }{
			{ID: p + "antimsg off", Text: "Deactivate"},
			{ID: p + "antimsg list", Text: "Target List"},
			{ID: p + "antimsg clear", Text: "Clear Targets"},
		}

		return sendInteractiveButtonsWithMentions(ctx, bodyText, fmt.Sprintf("%s AntiMsg Moderation", ctx.GetBotName()), buttons, addedMentions)
	}

	switch sub {
	case "on", "enable":
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return sendAntiMsgMenu(ctx, s, "AntiMsg has been activated for this group.")

	case "off", "disable":
		_ = s.PutSetting(ctx.Ctx, statusKey, "off")
		return sendAntiMsgMenu(ctx, s, "AntiMsg has been deactivated for this group.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, statusKey, "off")
			return sendAntiMsgMenu(ctx, s, "AntiMsg has been deactivated for this group.")
		}
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return sendAntiMsgMenu(ctx, s, "AntiMsg has been activated for this group.")

	case "add":
		if len(targets) == 0 {
			p := ctx.GetPrefix()
			return ctx.Reply(fmt.Sprintf("Please reply to a user's message or mention (@user) to add them to AntiMsg.\n\nExample:\n- Reply to message with `%santimsg`\n- `%santimsg @user`", p, p))
		}
		return nil

	case "del", "remove", "delete":
		if len(targets) == 0 {
			return ctx.Reply("Please reply to a user's message or mention (@user) to remove them from AntiMsg.")
		}
		rawUsers, _ := s.GetSetting(ctx.Ctx, usersKey)
		users := splitCSV(rawUsers)

		var removedMentions []types.JID
		var removedUsernames []string

		for _, t := range targets {
			tStr := t.ToNonAD().String()
			newUsers := make([]string, 0, len(users))
			for _, u := range users {
				if u != tStr {
					newUsers = append(newUsers, u)
				}
			}
			users = newUsers
			resolvedJID, username := ctx.ResolveMention(t)
			removedMentions = append(removedMentions, resolvedJID)
			removedUsernames = append(removedUsernames, "@"+username)
		}

		_ = s.PutSetting(ctx.Ctx, usersKey, strings.Join(users, ","))

		p := ctx.GetPrefix()
		bodyText := fmt.Sprintf("╭━━━〔 ANTIMSG UPDATED 〕━━━\n│ Removed: %s\n│ Total  : %d targeted user(s)\n╰━━━━━━━━━━━━━━━━━━━━━━", strings.Join(removedUsernames, ", "), len(users))

		status, _ := s.GetSetting(ctx.Ctx, statusKey)
		var toggleBtn struct{ ID, Text string }
		if status == "on" {
			toggleBtn = struct{ ID, Text string }{ID: p + "antimsg off", Text: "Deactivate"}
		} else {
			toggleBtn = struct{ ID, Text string }{ID: p + "antimsg on", Text: "Activate"}
		}

		buttons := []struct{ ID, Text string }{
			toggleBtn,
			{ID: p + "antimsg list", Text: "Target List"},
		}

		return sendInteractiveButtonsWithMentions(ctx, bodyText, fmt.Sprintf("%s AntiMsg Moderation", ctx.GetBotName()), buttons, removedMentions)

	case "list":
		rawUsers, _ := s.GetSetting(ctx.Ctx, usersKey)
		users := splitCSV(rawUsers)
		status, _ := s.GetSetting(ctx.Ctx, statusKey)
		if status == "" {
			status = "off"
		}

		p := ctx.GetPrefix()
		if len(users) == 0 {
			bodyText := fmt.Sprintf("╭━━━〔 ANTIMSG TARGETS 〕━━━\n│ Status: %s\n│ Targets: None\n╰━━━━━━━━━━━━━━━━━━━━━━\n\nNo participants are currently targeted in this group.\nReply to or mention (@user) anyone with %santimsg to add them.", strings.ToUpper(status), p)
			buttons := []struct{ ID, Text string }{
				{ID: p + "antimsg on", Text: "Activate"},
			}
			return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s AntiMsg Moderation", ctx.GetBotName()), buttons)
		}

		var sb strings.Builder
		var mentions []types.JID
		fmt.Fprintf(&sb, "╭━━━〔 ANTIMSG TARGETS 〕━━━\n│ Status : %s\n│ Total  : %d targeted user(s)\n╰━━━━━━━━━━━━━━━━━━━━━━\n\nTargeted Participants:\n", strings.ToUpper(status), len(users))

		for _, u := range users {
			uj, err := types.ParseJID(u)
			if err == nil {
				resolvedJID, username := ctx.ResolveMention(uj)
				fmt.Fprintf(&sb, "• @%s\n", username)
				mentions = append(mentions, resolvedJID)
			} else {
				fmt.Fprintf(&sb, "• %s\n", u)
			}
		}

		var toggleBtn struct{ ID, Text string }
		if status == "on" {
			toggleBtn = struct{ ID, Text string }{ID: p + "antimsg off", Text: "Deactivate"}
		} else {
			toggleBtn = struct{ ID, Text string }{ID: p + "antimsg on", Text: "Activate"}
		}

		buttons := []struct{ ID, Text string }{
			toggleBtn,
			{ID: p + "antimsg clear", Text: "Clear Targets"},
		}

		return sendInteractiveButtonsWithMentions(ctx, strings.TrimSpace(sb.String()), fmt.Sprintf("%s AntiMsg Moderation", ctx.GetBotName()), buttons, mentions)

	case "clear":
		_ = s.PutSetting(ctx.Ctx, usersKey, "")
		p := ctx.GetPrefix()
		bodyText := "AntiMsg target list cleared for this group."
		buttons := []struct{ ID, Text string }{
			{ID: p + "antimsg off", Text: "Deactivate"},
		}
		return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s AntiMsg Moderation", ctx.GetBotName()), buttons)

	default:
		// If no subcommands or unknown command, and status is currently off, activate it.
		currStatus, _ := s.GetSetting(ctx.Ctx, statusKey)
		if currStatus != "on" {
			_ = s.PutSetting(ctx.Ctx, statusKey, "on")
			return sendAntiMsgMenu(ctx, s, "AntiMsg has been activated for this group.")
		}
		return sendAntiMsgMenu(ctx, s, "")
	}
}

func sendAntiMsgMenu(ctx *Context, s *sqlstore.SQLStore, note string) error {
	chatKey := ctx.Chat.String()
	status, _ := s.GetSetting(ctx.Ctx, "antimsg_status:"+chatKey)
	if status == "" {
		status = "off"
	}

	rawUsers, _ := s.GetSetting(ctx.Ctx, "antimsg_users:"+chatKey)
	users := splitCSV(rawUsers)

	p := ctx.GetPrefix()
	var sb strings.Builder
	fmt.Fprintf(&sb, "╭━━━〔 ANTIMSG CONFIGURATION 〕━━━\n│ Group  : %s\n│ Status : %s\n│ Targets: %d user(s)\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n", chatKey, strings.ToUpper(status), len(users))

	if note != "" {
		sb.WriteString(note + "\n\n")
	}

	sb.WriteString("💡 *How to use AntiMsg:*\n")
	fmt.Fprintf(&sb, "• Reply to any message with `%santimsg` to add user\n", p)
	fmt.Fprintf(&sb, "• Mention `@user` with `%santimsg` to add user\n", p)
	fmt.Fprintf(&sb, "• Remove user: `%santimsg del @user`\n", p)
	fmt.Fprintf(&sb, "• View list: `%santimsg list`\n", p)

	var actionButton struct{ ID, Text string }
	if status == "on" {
		actionButton = struct{ ID, Text string }{ID: p + "antimsg off", Text: "Deactivate"}
	} else {
		actionButton = struct{ ID, Text string }{ID: p + "antimsg on", Text: "Activate"}
	}

	buttons := []struct{ ID, Text string }{
		actionButton,
		{ID: p + "antimsg list", Text: "Target List"},
		{ID: p + "antimsg clear", Text: "Clear Targets"},
	}

	return sendInteractiveButtons(ctx, strings.TrimSpace(sb.String()), fmt.Sprintf("%s AntiMsg Moderation", ctx.GetBotName()), buttons)
}

func extractTargetParticipants(ctx *Context, args []string) []types.JID {
	var targets []types.JID
	seen := make(map[string]bool)

	addJID := func(j types.JID) {
		if j.IsEmpty() {
			return
		}
		nonAD := j.ToNonAD()
		key := nonAD.String()
		if !seen[key] {
			seen[key] = true
			targets = append(targets, nonAD)
		}
	}

	// 1. Quoted message sender
	if quotedSender, ok := ctx.GetQuotedSender(); ok && !quotedSender.IsEmpty() {
		addJID(quotedSender)
	}

	// 2. Mentioned JIDs
	if ctx.Evt != nil && ctx.Evt.Message != nil {
		if ext := ctx.Evt.Message.GetExtendedTextMessage(); ext != nil {
			if ci := ext.GetContextInfo(); ci != nil {
				for _, m := range ci.GetMentionedJID() {
					parsed, err := types.ParseJID(m)
					if err == nil && !parsed.IsEmpty() {
						addJID(parsed)
					}
				}
			}
		}
	}

	// 3. Command arguments (@user or phone numbers)
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" || strings.EqualFold(arg, "add") || strings.EqualFold(arg, "del") ||
			strings.EqualFold(arg, "remove") || strings.EqualFold(arg, "delete") ||
			strings.EqualFold(arg, "on") || strings.EqualFold(arg, "off") ||
			strings.EqualFold(arg, "toggle") || strings.EqualFold(arg, "list") ||
			strings.EqualFold(arg, "clear") || strings.EqualFold(arg, "enable") ||
			strings.EqualFold(arg, "disable") {
			continue
		}
		raw := strings.TrimPrefix(arg, "@")
		if !strings.Contains(raw, "@") {
			raw = raw + "@s.whatsapp.net"
		}
		parsed, err := types.ParseJID(raw)
		if err == nil && !parsed.IsEmpty() {
			addJID(parsed)
		}
	}

	return targets
}
