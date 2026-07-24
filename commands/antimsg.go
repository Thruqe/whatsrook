// AntiMsg command – automatically delete incoming messages from specified group participants.
package commands

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
		status, _ := s.GetSetting(ctx.Ctx, statusKey)
		if status == "" {
			status = "off"
		}
		rawUsers, _ := s.GetSetting(ctx.Ctx, usersKey)
		users := splitCSV(rawUsers)
		userCount := len(users)

		return ctx.Reply(fmt.Sprintf("AntiMsg Status: %s\nTargeted Participants: %d\n\nUsage: .antimsg [on|off|add|del|list|clear]", strings.ToUpper(status), userCount))
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

	case "add":
		targetJID := extractTargetParticipant(ctx, args)
		if targetJID.IsEmpty() {
			return ctx.Reply("Please mention a participant, quote their message, or specify their JID/phone number to add to AntiMsg.")
		}
		targetStr := targetJID.ToNonAD().String()
		rawUsers, _ := s.GetSetting(ctx.Ctx, usersKey)
		users := splitCSV(rawUsers)
		if !containsString(users, targetStr) {
			users = append(users, targetStr)
		}
		_ = s.PutSetting(ctx.Ctx, usersKey, strings.Join(users, ","))
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply("Added " + targetStr + " to AntiMsg target list.")

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
		return ctx.Reply("Removed " + targetStr + " from AntiMsg target list.")

	case "list":
		rawUsers, _ := s.GetSetting(ctx.Ctx, usersKey)
		users := splitCSV(rawUsers)
		if len(users) == 0 {
			return ctx.Reply("No participants are currently targeted by AntiMsg in this group.")
		}
		var sb strings.Builder
		sb.WriteString("AntiMsg Targeted Participants:\n")
		for i, u := range users {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, u))
		}
		return ctx.Reply(strings.TrimSpace(sb.String()))

	case "clear":
		_ = s.PutSetting(ctx.Ctx, usersKey, "")
		return ctx.Reply("Cleared AntiMsg target list for this group.")

	default:
		return ctx.Reply("Usage: .antimsg [on|off|add|del|list|clear]")
	}
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
