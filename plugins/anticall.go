// AntiCall command – configure call rejection rules, contact filters, country code filters, and spam warning thresholds.
package plugins

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"whatsrook/store/sqlstore"
)

func init() {
	Register(&Command{
		Name:        "anticall",
		Aliases:     []string{"acall"},
		Description: "Configure call rejection rules, contacts filter, allowed country codes, and call warning thresholds",
		Category:    "calls",
		IsPublic:    false,
		Handler:     handleAntiCall,
	})
}

func handleAntiCall(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	p := ctx.GetPrefix()
	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		return sendAntiCallMenu(ctx, s)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable":
		_ = s.PutSetting(ctx.Ctx, "anticall_status", "on")
		return ctx.Reply("AntiCall enabled.")

	case "off", "disable":
		_ = s.PutSetting(ctx.Ctx, "anticall_status", "off")
		return ctx.Reply("AntiCall disabled.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, "anticall_status")
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, "anticall_status", "off")
			return ctx.Reply("AntiCall disabled.")
		}
		_ = s.PutSetting(ctx.Ctx, "anticall_status", "on")
		return ctx.Reply("AntiCall enabled.")

	case "customize", "custom", "help":
		return sendAntiCallCustomizeGuide(ctx)

	case "contacts":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, "anticall_contacts_only")
			return ctx.Reply("AntiCall contacts only setting: " + curr)
		}
		mode := strings.ToLower(args[1])
		switch mode {
		case "on", "true":
			_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "true")
			return ctx.Reply("AntiCall set to allow calls from contacts only.")
		case "off", "false":
			_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "false")
			return ctx.Reply("AntiCall contacts only restriction disabled.")
		case "toggle":
			curr, _ := s.GetSetting(ctx.Ctx, "anticall_contacts_only")
			if curr == "true" {
				_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "false")
				return ctx.Reply("AntiCall contacts only restriction disabled.")
			}
			_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "true")
			return ctx.Reply(fmt.Sprintf("AntiCall set to allow calls from contacts only."))
		}
		return ctx.Reply(fmt.Sprintf("Usage: %santicall contacts [on|off|toggle]", p))

	case "cc":
		if len(args) < 2 {
			allowed, _ := s.GetSetting(ctx.Ctx, "anticall_allowed_cc")
			if allowed == "" {
				allowed = "none"
			}
			return ctx.Reply("Allowed country codes: " + allowed)
		}
		action := strings.ToLower(args[1])
		switch action {
		case "add":
			if len(args) < 3 {
				return ctx.Reply(fmt.Sprintf("Usage: %santicall cc add <country_code>", p))
			}
			cc := strings.TrimPrefix(args[2], "+")
			allowed, _ := s.GetSetting(ctx.Ctx, "anticall_allowed_cc")
			codes := splitCSV(allowed)
			if !containsString(codes, cc) {
				codes = append(codes, cc)
			}
			_ = s.PutSetting(ctx.Ctx, "anticall_allowed_cc", strings.Join(codes, ","))
			return ctx.Reply("Added country code +" + cc + " to allowed list.")

		case "del", "remove":
			if len(args) < 3 {
				return ctx.Reply(fmt.Sprintf("Usage: %santicall cc del <country_code>", p))
			}
			cc := strings.TrimPrefix(args[2], "+")
			allowed, _ := s.GetSetting(ctx.Ctx, "anticall_allowed_cc")
			codes := splitCSV(allowed)
			newCodes := make([]string, 0, len(codes))
			for _, c := range codes {
				if c != cc {
					newCodes = append(newCodes, c)
				}
			}
			_ = s.PutSetting(ctx.Ctx, "anticall_allowed_cc", strings.Join(newCodes, ","))
			return ctx.Reply("Removed country code +" + cc + " from allowed list.")

		case "clear":
			_ = s.PutSetting(ctx.Ctx, "anticall_allowed_cc", "")
			return ctx.Reply("Cleared allowed country codes list.")

		default:
			return ctx.Reply(fmt.Sprintf("Usage: %santicall cc [add|del|clear]", p))
		}

	case "warn", "warnings":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, "anticall_max_warn")
			if curr == "" {
				curr = "3"
			}
			return ctx.Reply("Current call warning threshold: " + curr)
		}
		num, err := strconv.Atoi(args[1])
		if err != nil || num < 1 {
			return ctx.Reply("Please specify a valid warning count number (e.g. 3).")
		}
		_ = s.PutSetting(ctx.Ctx, "anticall_max_warn", strconv.Itoa(num))
		return ctx.Reply("Call warning threshold set to " + strconv.Itoa(num))

	default:
		return ctx.Reply(fmt.Sprintf("Usage: %santicall [on|off|toggle|customize|contacts|cc|warn]", p))
	}
}

func sendAntiCallMenu(ctx *Context, s *sqlstore.SQLStore) error {
	status, _ := s.GetSetting(ctx.Ctx, "anticall_status")
	if status == "" {
		status = "off"
	}

	p := ctx.GetPrefix()
	bodyText := fmt.Sprintf("╭━━━〔 ANTICALL CONFIGURATION 〕━━━\n│ Status : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nChoose an option below to change status or view customization options.", strings.ToUpper(status))

	var actionButton struct{ ID, Text string }
	if status == "on" {
		actionButton = struct{ ID, Text string }{ID: p + "anticall off", Text: "Deactivate"}
	} else {
		actionButton = struct{ ID, Text string }{ID: p + "anticall on", Text: "Activate"}
	}

	buttons := []struct{ ID, Text string }{
		actionButton,
		{ID: p + "anticall customize", Text: "Customize"},
	}

	return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s AntiCall Rejection", ctx.GetBotName()), buttons)
}

func sendAntiCallCustomizeGuide(ctx *Context) error {
	p := ctx.GetPrefix()
	var sb strings.Builder
	sb.WriteString("╭━━━〔 ANTICALL CUSTOMIZATION GUIDE 〕━━━\n\n")
	sb.WriteString("Available Customizations:\n")
	fmt.Fprintf(&sb, "• Contacts Only Restriction : `%santicall contacts on | off`\n", p)
	fmt.Fprintf(&sb, "• Country Code Whitelist    : `%santicall cc add | del | clear <code >`\n", p)
	fmt.Fprintf(&sb, "• Max Warning Threshold     : `%santicall warn <number>`\n\n", p)

	sb.WriteString("Examples:\n")
	fmt.Fprintf(&sb, "1. `%santicall contacts on` (Reject calls from non-contacts)\n", p)
	fmt.Fprintf(&sb, "2. `%santicall cc add 234` (Allow calls from country code +234)\n", p)
	fmt.Fprintf(&sb, "3. `%santicall warn 3` (Set warning limit before auto-block to 3)\n", p)

	return ctx.Reply(strings.TrimSpace(sb.String()))
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func containsString(slice []string, val string) bool {
	return slices.Contains(slice, val)
}
