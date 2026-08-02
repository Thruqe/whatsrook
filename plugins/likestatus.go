// LikeStatus command – automatically reacts to incoming WhatsApp status broadcasts with a random love emoji.
package commands

import (
	"fmt"
	"strings"

	"whatsrook/store/sqlstore"
)

func init() {
	Register(&Command{
		Name:        "likestatus",
		Aliases:     []string{"autolike", "likestatuses", "lovestatus"},
		Description: "Automatically react with love emojis to incoming status broadcasts",
		Category:    "settings",
		IsPublic:    false,
		Handler:     handleLikeStatusCmd,
	})
}

func handleLikeStatusCmd(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to bot owner and sudoers.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	statusKey := "likestatus_status"

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		return sendLikeStatusMenu(ctx, s)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable":
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply("LikeStatus ENABLED. The bot will automatically react to status broadcasts with love emojis.")

	case "off", "disable":
		_ = s.PutSetting(ctx.Ctx, statusKey, "off")
		return ctx.Reply("LikeStatus DISABLED.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, statusKey, "off")
			return ctx.Reply("LikeStatus DISABLED.")
		}
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply("LikeStatus ENABLED.")

	case "customize", "custom", "help":
		return sendLikeStatusCustomizeGuide(ctx)

	default:
		return ctx.Reply("Usage: .likestatus [on|off|toggle|customize]")
	}
}

func sendLikeStatusMenu(ctx *Context, s *sqlstore.SQLStore) error {
	status, _ := s.GetSetting(ctx.Ctx, "likestatus_status")
	if status == "" {
		status = "off"
	}

	p := ctx.GetPrefix()
	bodyText := fmt.Sprintf("╭━━━〔 LIFESTATUS AUTO-REACTION 〕━━━\n│ Status : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nAutomatically reacts to status broadcasts with random love emojis (❤️, 💕, 💖, 💗, 💓, 💞, 💘, 💌, 🥰, 😍).", strings.ToUpper(status))

	var actionButton struct{ ID, Text string }
	if status == "on" {
		actionButton = struct{ ID, Text string }{ID: p + "likestatus off", Text: "Deactivate"}
	} else {
		actionButton = struct{ ID, Text string }{ID: p + "likestatus on", Text: "Activate"}
	}

	buttons := []struct{ ID, Text string }{
		actionButton,
		{ID: p + "likestatus customize", Text: "Customize"},
	}

	return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s LikeStatus", ctx.GetBotName()), buttons)
}

func sendLikeStatusCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	var sb strings.Builder
	sb.WriteString("╭━━━〔 LIKESTATUS CUSTOMIZATION GUIDE 〕━━━\n\n")
	sb.WriteString("Description:\n")
	sb.WriteString("When enabled, WhatsRook will automatically react to every incoming status/story broadcast with a randomly selected love emoji.\n\n")

	sb.WriteString("Commands:\n")
	fmt.Fprintf(&sb, "• Enable Auto-Like  : `%slikestatus on`\n", p)
	fmt.Fprintf(&sb, "• Disable Auto-Like : `%slikestatus off`\n", p)
	fmt.Fprintf(&sb, "• Toggle Status     : `%slikestatus toggle`\n", p)

	return ctx.Reply(strings.TrimSpace(sb.String()))
}
