// Events command – toggle real-time WhatsApp group event notification messages.
package plugins

import (
	"fmt"
	"strings"

	"whatsrook/store/sqlstore"
)

func init() {
	Register(&Command{
		Name:        "events",
		Aliases:     []string{"groupevents", "eventnotify"},
		Description: "Toggle real-time notifications for group subject, description, settings, and participant changes",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleEventsCmd,
	})
}

func handleEventsCmd(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	chatKey := ctx.Chat.String()
	statusKey := "events_status:" + chatKey

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		return sendEventsMenu(ctx, s)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable":
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply("Group Events notifications ENABLED.")

	case "off", "disable":
		_ = s.PutSetting(ctx.Ctx, statusKey, "off")
		return ctx.Reply("Group Events notifications DISABLED.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, statusKey, "off")
			return ctx.Reply("Group Events notifications DISABLED.")
		}
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply("Group Events notifications ENABLED.")

	case "customize", "custom", "help":
		return sendEventsCustomizeGuide(ctx)

	default:
		return ctx.Reply(fmt.Sprintf("Usage: %sevents [on|off|toggle|customize]", ctx.GetPrefix()))
	}
}

func sendEventsMenu(ctx *Context, s *sqlstore.SQLStore) error {
	chatKey := ctx.Chat.String()
	status, _ := s.GetSetting(ctx.Ctx, "events_status:"+chatKey)
	if status == "" {
		status = "off"
	}

	p := ctx.GetPrefix()
	bodyText := fmt.Sprintf("╭━━━〔 GROUP EVENTS NOTIFICATIONS 〕━━━\n│ Status : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nChoose an option below to toggle notifications or view customization options.", strings.ToUpper(status))

	var actionButton struct{ ID, Text string }
	if status == "on" {
		actionButton = struct{ ID, Text string }{ID: p + "events off", Text: "Deactivate"}
	} else {
		actionButton = struct{ ID, Text string }{ID: p + "events on", Text: "Activate"}
	}

	buttons := []struct{ ID, Text string }{
		actionButton,
		{ID: p + "events customize", Text: "Customize"},
	}

	return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s Group Events", ctx.GetBotName()), buttons)
}

func sendEventsCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	var sb strings.Builder
	sb.WriteString("╭━━━〔 GROUP EVENTS NOTIFICATIONS GUIDE 〕━━━\n\n")
	sb.WriteString("Supported Event Notifications:\n")
	sb.WriteString("• Group Name / Subject Changes\n")
	sb.WriteString("• Group Description / Topic Updates\n")
	sb.WriteString("• Group Settings Lock (Admins vs All Members)\n")
	sb.WriteString("• Group Announce Mute (Admins vs All Members)\n")
	sb.WriteString("• Admin Promotions & Demotions\n")
	sb.WriteString("• Member Joins & Leaves\n\n")

	sb.WriteString("Commands:\n")
	fmt.Fprintf(&sb, "• Enable Notifications  : `%sevents on`\n", p)
	fmt.Fprintf(&sb, "• Disable Notifications : `%sevents off`\n", p)
	fmt.Fprintf(&sb, "• Toggle Status        : `%sevents toggle`\n", p)

	return ctx.Reply(strings.TrimSpace(sb.String()))
}
