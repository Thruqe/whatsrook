// Welcome and Goodbye commands – configure group join/leave greetings with buttons, tags, group descriptions, custom text templates, and media.
package plugins

import (
	"fmt"
	"strings"
	"unicode"

	"whatsrook/store/sqlstore"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var titleCaser = cases.Title(language.English)

func init() {
	Register(&Command{
		Name:        "welcome",
		Aliases:     []string{"welc"},
		Description: "Configure group welcome messages, tagging, description headers, custom templates, and media",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleWelcome,
	})

	Register(&Command{
		Name:        "goodbye",
		Aliases:     []string{"bye"},
		Description: "Configure group goodbye messages, tagging, description headers, custom templates, and media",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    false,
		Handler:     handleGoodbye,
	})
}

func handleWelcome(ctx *Context) error {
	return handleGroupGreetingConfig(ctx, "welcome")
}

func handleGoodbye(ctx *Context) error {
	return handleGroupGreetingConfig(ctx, "goodbye")
}

func handleGroupGreetingConfig(ctx *Context, kind string) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Database store not available.")
	}

	chatKey := ctx.Chat.String()
	key := func(suffix string) string {
		return kind + "_" + suffix + ":" + chatKey
	}
	statusKey := key("status")
	tagKey := key("tag")
	descKey := key("desc")
	msgKey := key("msg")
	mediaKey := key("media")

	label := titleCase(kind)

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		return sendGreetingMenu(ctx, s, kind)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable":
		return applyToggle(ctx, s, statusKey, "on", label+" message")

	case "off", "disable":
		return applyToggle(ctx, s, statusKey, "off", label+" message")

	case "toggle":
		return applyToggle(ctx, s, statusKey, "toggle", label+" message")

	case "customize", "custom", "help":
		return sendGreetingCustomizeGuide(ctx, kind)

	case "tag":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, tagKey)
			return ctx.Reply(label + " participant tag setting: " + curr)
		}
		mode := strings.ToLower(args[1])
		if mode != "on" && mode != "true" && mode != "off" && mode != "false" && mode != "toggle" {
			return ctx.Reply("Usage: ." + kind + " tag [on|off|toggle]")
		}
		return applyToggle(ctx, s, tagKey, mode, label+" participant tagging")

	case "desc":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, descKey)
			return ctx.Reply(label + " group description setting: " + curr)
		}
		mode := strings.ToLower(args[1])
		if mode != "on" && mode != "true" && mode != "off" && mode != "false" && mode != "toggle" {
			return ctx.Reply("Usage: ." + kind + " desc [on|off|toggle]")
		}
		return applyToggle(ctx, s, descKey, mode, label+" group description inclusion")

	case "msg", "message", "text":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, msgKey)
			if curr == "" {
				curr = "none (using default template)"
			}
			return ctx.Reply(label + " custom message template: " + curr)
		}
		text := strings.TrimSpace(ctx.RawArgs[len(args[0]):])
		if err := s.PutSetting(ctx.Ctx, msgKey, text); err != nil {
			return ctx.Reply("Failed to update message template: " + err.Error())
		}
		return ctx.Reply(label + " custom message template updated.")

	case "media", "video":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, mediaKey)
			if curr == "" {
				curr = "none"
			}
			return ctx.Reply(label + " media URL: " + curr)
		}
		url := strings.TrimSpace(args[1])
		if url == "none" || url == "clear" {
			if err := s.PutSetting(ctx.Ctx, mediaKey, ""); err != nil {
				return ctx.Reply("Failed to clear media: " + err.Error())
			}
			return ctx.Reply(label + " media cleared.")
		}
		if err := s.PutSetting(ctx.Ctx, mediaKey, url); err != nil {
			return ctx.Reply("Failed to update media: " + err.Error())
		}
		return ctx.Reply(label + " media URL saved.")

	default:
		return ctx.Reply("Usage: ." + kind + " [on|off|toggle|customize|tag|desc|msg|media]")
	}
}

// applyToggle sets key to on/off, or flips its current value when mode is "toggle".
func applyToggle(ctx *Context, s *sqlstore.SQLStore, key, mode, label string) error {
	next := "on"
	switch mode {
	case "on", "true":
		next = "on"
	case "off", "false":
		next = "off"
	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, key)
		next = "on"
		if curr == "on" {
			next = "off"
		}
	}

	if err := s.PutSetting(ctx.Ctx, key, next); err != nil {
		return ctx.Reply("Failed to update setting: " + err.Error())
	}

	verb := "enabled"
	if next == "off" {
		verb = "disabled"
	}
	return ctx.Reply(label + " " + verb + ".")
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func sendGreetingMenu(ctx *Context, s *sqlstore.SQLStore, kind string) error {
	chatKey := ctx.Chat.String()
	groupName := chatKey
	if info, err := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat); err == nil && info != nil && info.GroupName.Name != "" {
		groupName = info.GroupName.Name
	}

	status, _ := s.GetSetting(ctx.Ctx, kind+"_status:"+chatKey)
	if status == "" {
		status = "off"
	}

	p := ctx.GetPrefix()
	bodyText := fmt.Sprintf("╭━━━〔 %s CONFIGURATION 〕━━━\n│ Group  : %s\n│ Status : %s\n╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\nChoose an option below to change status or view customization options.", strings.ToUpper(kind), groupName, strings.ToUpper(status))

	var actionButton struct{ ID, Text string }
	if status == "on" {
		actionButton = struct{ ID, Text string }{ID: p + kind + " off", Text: "Deactivate"}
	} else {
		actionButton = struct{ ID, Text string }{ID: p + kind + " on", Text: "Activate"}
	}

	buttons := []struct{ ID, Text string }{
		actionButton,
		{ID: p + kind + " customize", Text: "Customize"},
	}

	return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s %s Moderation", ctx.GetBotName(), titleCase(kind)), buttons)
}

func sendGreetingCustomizeGuide(ctx *Context, kind string) error {
	p := ctx.GetPrefix()
	kUpper := strings.ToUpper(kind)

	var sb strings.Builder
	fmt.Fprintf(&sb, "╭━━━〔 %s CUSTOMIZATION GUIDE 〕━━━\n\n", kUpper)
	sb.WriteString("Available Customizations:\n")
	fmt.Fprintf(&sb, "• Custom Message : `%s%s msg <your message text>`\n", p, kind)
	fmt.Fprintf(&sb, "• Participant Tagging : `%s%s tag on | off`\n", p, kind)
	fmt.Fprintf(&sb, "• Group Description   : `%s%s desc on | off`\n", p, kind)
	fmt.Fprintf(&sb, "• Greeting Media URL  : `%s%s media <url | clear>`\n\n", p, kind)

	sb.WriteString("Available GroupInfo Placeholders:\n")
	sb.WriteString("- `{user}`       : Participant mention tag (@username)\n")
	sb.WriteString("- `{user_id}`    : Participant's phone number / user ID\n")
	sb.WriteString("- `{user_jid}`   : Participant's full WhatsApp JID\n")
	sb.WriteString("- `{group}`      : Group Name\n")
	sb.WriteString("- `{group_jid}`  : Group JID\n")
	sb.WriteString("- `{desc}`       : Group Description / Topic\n")
	sb.WriteString("- `{members}`    : Total group participant count\n")
	sb.WriteString("- `{admins}`     : Total group admin count\n")
	sb.WriteString("- `{owner}`      : Mentions group creator / owner\n")
	sb.WriteString("- `{created_at}` : Group creation date\n\n")

	sb.WriteString("Examples:\n")
	if kind == "welcome" {
		fmt.Fprintf(&sb, "1. `%swelcome msg Welcome {user} to {group}! We now have {members} members (Admins: {admins}). Created by {owner} on {created_at}.`\n", p)
		fmt.Fprintf(&sb, "2. `%swelcome tag on`\n", p)
		fmt.Fprintf(&sb, "3. `%swelcome media https://example.com/welcome.mp4`\n", p)
	} else {
		fmt.Fprintf(&sb, "1. `%sgoodbye msg Goodbye {user}! {group} now has {members} members remaining.`\n", p)
		fmt.Fprintf(&sb, "2. `%sgoodbye tag off`\n", p)
		fmt.Fprintf(&sb, "3. `%sgoodbye media https://example.com/goodbye.gif`\n", p)
	}

	return ctx.Reply(strings.TrimSpace(sb.String()))
}
