// AntiSpam command – configure group anti-spam rules, message rate limits, and automated actions.
package commands

import (
	"fmt"
	"strconv"
	"strings"

	"whatsrook/store/sqlstore"
)

func init() {
	Register(&Command{
		Name:        "antispam",
		Aliases:     []string{"anti-spam", "aspam"},
		Description: "Configure group anti-spam rate limits, warning thresholds, and automated actions",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleAntiSpam,
	})
}

func handleAntiSpam(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	chatKey := ctx.Chat.String()
	statusKey := "antispam_status:" + chatKey
	actionKey := "antispam_action:" + chatKey
	maxKey := "antispam_max:" + chatKey

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		return sendAntiSpamMenu(ctx, s)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable":
		if err := s.PutSetting(ctx.Ctx, statusKey, "on"); err != nil {
			return ctx.Reply("Failed to enable AntiSpam.")
		}
		return ctx.Reply("AntiSpam feature enabled for this group.")

	case "off", "disable":
		if err := s.PutSetting(ctx.Ctx, statusKey, "off"); err != nil {
			return ctx.Reply("Failed to disable AntiSpam.")
		}
		return ctx.Reply("AntiSpam feature disabled for this group.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		nextState := "on"
		if curr == "on" {
			nextState = "off"
		}
		if err := s.PutSetting(ctx.Ctx, statusKey, nextState); err != nil {
			return ctx.Reply("Failed to toggle AntiSpam.")
		}
		if nextState == "on" {
			return ctx.Reply("AntiSpam feature enabled for this group.")
		}
		return ctx.Reply("AntiSpam feature disabled for this group.")

	case "customize", "custom", "help":
		return sendAntiSpamCustomizeGuide(ctx)

	case "action":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, actionKey)
			if curr == "" {
				curr = "delete"
			}
			return ctx.Reply("Current AntiSpam action: " + curr + "\nUsage: .antispam action [delete|warn|kick]")
		}
		act := strings.ToLower(args[1])
		if act != "delete" && act != "warn" && act != "kick" {
			return ctx.Reply("Invalid action. Usage: .antispam action [delete|warn|kick]")
		}
		if err := s.PutSetting(ctx.Ctx, actionKey, act); err != nil {
			return ctx.Reply("Failed to update AntiSpam action.")
		}
		return ctx.Reply("AntiSpam action updated to " + act + ".")

	case "max", "threshold", "limit":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, maxKey)
			if curr == "" {
				curr = "5"
			}
			return ctx.Reply("Current AntiSpam message limit: " + curr + " msgs/5s\nUsage: .antispam max [number]")
		}
		num, err := strconv.Atoi(args[1])
		if err != nil || num < 2 || num > 30 {
			return ctx.Reply("Please specify a valid message limit between 2 and 30.")
		}
		if err := s.PutSetting(ctx.Ctx, maxKey, strconv.Itoa(num)); err != nil {
			return ctx.Reply("Failed to update AntiSpam threshold.")
		}
		return ctx.Reply("AntiSpam message limit set to " + strconv.Itoa(num) + " messages per 5 seconds.")

	default:
		return ctx.Reply("Usage: .antispam [on|off|toggle|customize|action|max]")
	}
}

func sendAntiSpamMenu(ctx *Context, s *sqlstore.SQLStore) error {
	chatKey := ctx.Chat.String()
	groupName := chatKey
	if info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat); err == nil && info != nil && info.GroupName.Name != "" {
		groupName = info.GroupName.Name
	}

	status, _ := s.GetSetting(ctx.Ctx, "antispam_status:"+chatKey)
	if status == "" {
		status = "off"
	}

	p := ctx.GetPrefix()
	bodyText := fmt.Sprintf("╭━━━〔 ANTISPAM CONFIGURATION 〕━━━\n│ Group  : %s\n│ Status : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nChoose an option below to change status or view customization options.", groupName, strings.ToUpper(status))

	var actionButton struct{ ID, Text string }
	if status == "on" {
		actionButton = struct{ ID, Text string }{ID: p + "antispam off", Text: "Deactivate"}
	} else {
		actionButton = struct{ ID, Text string }{ID: p + "antispam on", Text: "Activate"}
	}

	buttons := []struct{ ID, Text string }{
		actionButton,
		{ID: p + "antispam customize", Text: "Customize"},
	}

	return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s AntiSpam Moderation", ctx.GetBotName()), buttons)
}

func sendAntiSpamCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	var sb strings.Builder
	sb.WriteString("╭━━━〔 ANTISPAM CUSTOMIZATION GUIDE 〕━━━\n\n")
	sb.WriteString("Available Customizations:\n")
	fmt.Fprintf(&sb, "• Automated Action : `%santispam action delete | warn | kick`\n", p)
	fmt.Fprintf(&sb, "• Rate Limit Max   : `%santispam max <number>` (messages per 5 seconds)\n\n", p)

	sb.WriteString("Examples:\n")
	fmt.Fprintf(&sb, "1. `%santispam action kick` (Automatically kick spammers)\n", p)
	fmt.Fprintf(&sb, "2. `%santispam action warn` (Issue warnings to spammers)\n", p)
	fmt.Fprintf(&sb, "3. `%santispam max 3` (Set limit to 3 msgs / 5s)\n", p)

	return ctx.Reply(strings.TrimSpace(sb.String()))
}
