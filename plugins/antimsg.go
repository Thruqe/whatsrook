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
	if len(args) == 0 {
		return sendAntiMsgMenu(ctx, s)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable":
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply("AntiMsg feature enabled for this group.")

	case "off", "disable":
		_ = s.PutSetting(ctx.Ctx, statusKey, "off")
		return ctx.Reply("AntiMsg feature disabled for this group.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, statusKey, "off")
			return ctx.Reply("AntiMsg feature disabled for this group.")
		}
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply("AntiMsg feature enabled for this group.")

	case "customize", "custom", "help":
		return sendAntiMsgCustomizeGuide(ctx)

	case "add":
		targetJID := extractTargetParticipant(ctx, args)
		if targetJID.IsEmpty() {
			p := ctx.GetPrefix()
			return ctx.Reply(fmt.Sprintf("Usage:\n- %santimsg add @user\n- %santimsg add 1234567890\n- Reply to a user's message with %santimsg add", p, p, p))
		}
		targetStr := targetJID.ToNonAD().String()
		rawUsers, _ := s.GetSetting(ctx.Ctx, usersKey)
		users := splitCSV(rawUsers)
		if !containsString(users, targetStr) {
			users = append(users, targetStr)
		}
		_ = s.PutSetting(ctx.Ctx, usersKey, strings.Join(users, ","))
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		resolvedJID, username := ctx.ResolveMention(targetJID)
		return ctx.ReplyWithMentions(fmt.Sprintf(" Added @%s to AntiMsg target list.", username), []types.JID{resolvedJID})

	case "del", "remove":
		targetJID := extractTargetParticipant(ctx, args)
		if targetJID.IsEmpty() {
			return ctx.Reply("Please mention a participant, quote their message, or specify their JID/phone number to remove from AntiMsg.")
		}
		targetStr := targetJID.ToNonAD().String()
		rawUsers, _ := s.GetSetting(ctx.Ctx, usersKey)
		users := splitCSV(rawUsers)
		newUsers := make([]string, 0, len(users))
		for _, u := range users {
			if u != targetStr {
				newUsers = append(newUsers, u)
			}
		}
		_ = s.PutSetting(ctx.Ctx, usersKey, strings.Join(newUsers, ","))
		resolvedJID, username := ctx.ResolveMention(targetJID)
		return ctx.ReplyWithMentions(fmt.Sprintf(" Removed @%s from AntiMsg target list.", username), []types.JID{resolvedJID})

	case "list":
		rawUsers, _ := s.GetSetting(ctx.Ctx, usersKey)
		users := splitCSV(rawUsers)
		if len(users) == 0 {
			return ctx.Reply("No participants are currently targeted by AntiMsg in this group.")
		}
		var sb strings.Builder
		var mentions []types.JID
		sb.WriteString("AntiMsg Targeted Participants:\n\n")
		for _, u := range users {
			uj, err := types.ParseJID(u)
			if err == nil {
				resolvedJID, username := ctx.ResolveMention(uj)
				fmt.Fprintf(&sb, "- @%s\n", username)
				mentions = append(mentions, resolvedJID)
			} else {
				fmt.Fprintf(&sb, "- %s\n", u)
			}
		}
		return ctx.ReplyWithMentions(strings.TrimSpace(sb.String()), mentions)

	case "clear":
		_ = s.PutSetting(ctx.Ctx, usersKey, "")
		return ctx.Reply("Cleared AntiMsg target list for this group.")

	default:
		return ctx.Reply(fmt.Sprintf("Usage: %santimsg [on|off|toggle|customize|add|del|list|clear]", ctx.GetPrefix()))
	}
}

func sendAntiMsgMenu(ctx *Context, s *sqlstore.SQLStore) error {
	chatKey := ctx.Chat.String()
	status, _ := s.GetSetting(ctx.Ctx, "antimsg_status:"+chatKey)
	if status == "" {
		status = "off"
	}

	p := ctx.GetPrefix()
	bodyText := fmt.Sprintf("╭━━━〔 ANTIMSG CONFIGURATION 〕━━━\n│ Group  : %s\n│ Status : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nChoose an option below to change status or view customization options.", chatKey, strings.ToUpper(status))

	var actionButton struct{ ID, Text string }
	if status == "on" {
		actionButton = struct{ ID, Text string }{ID: p + "antimsg off", Text: "Deactivate"}
	} else {
		actionButton = struct{ ID, Text string }{ID: p + "antimsg on", Text: "Activate"}
	}

	buttons := []struct{ ID, Text string }{
		actionButton,
		{ID: p + "antimsg customize", Text: "Customize"},
	}

	return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s AntiMsg Moderation", ctx.GetBotName()), buttons)
}

func sendAntiMsgCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	var sb strings.Builder
	sb.WriteString("╭━━━〔 ANTIMSG CUSTOMIZATION GUIDE 〕━━━\n\n")
	sb.WriteString("Available Customizations:\n")
	fmt.Fprintf(&sb, "• Target User   : `%santimsg add @user` (or reply to user's message)\n", p)
	fmt.Fprintf(&sb, "• Remove User   : `%santimsg del @user`\n", p)
	fmt.Fprintf(&sb, "• View Targets  : `%santimsg list`\n", p)
	fmt.Fprintf(&sb, "• Clear Targets : `%santimsg clear`\n\n", p)

	sb.WriteString("Examples:\n")
	fmt.Fprintf(&sb, "1. `%santimsg add @user` (Auto-delete messages from mentioned user)\n", p)
	fmt.Fprintf(&sb, "2. `%santimsg del @user` (Remove user from auto-deletion list)\n", p)
	fmt.Fprintf(&sb, "3. `%santimsg list` (View all targeted users)\n", p)

	return ctx.Reply(strings.TrimSpace(sb.String()))
}

func extractTargetParticipant(ctx *Context, args []string) types.JID {
	if quotedSender, ok := ctx.GetQuotedSender(); ok && !quotedSender.IsEmpty() {
		return quotedSender
	}
	if len(ctx.Evt.Message.GetExtendedTextMessage().GetContextInfo().GetMentionedJID()) > 0 {
		for _, m := range ctx.Evt.Message.GetExtendedTextMessage().GetContextInfo().GetMentionedJID() {
			parsed, err := types.ParseJID(m)
			if err == nil && !parsed.IsEmpty() {
				return parsed
			}
		}
	}
	if len(args) > 1 {
		raw := strings.TrimPrefix(args[1], "@")
		if !strings.Contains(raw, "@") {
			raw = raw + "@s.whatsapp.net"
		}
		parsed, err := types.ParseJID(raw)
		if err == nil && !parsed.IsEmpty() {
			return parsed
		}
	}
	return types.EmptyJID
}
