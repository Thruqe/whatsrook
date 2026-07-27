// Welcome and Goodbye commands – configure group join/leave greetings with buttons, tags, group descriptions, custom text templates, and media.
package commands

import (
	"fmt"
	"strings"
	"unicode"

	"whatsrook/store/sqlstore"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
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
		return ctx.Reply("Usage: ." + kind + " [on|off|toggle|tag|desc|msg|media]")
	}
}

// applyToggle sets key to on/off, or flips its current value when mode is "toggle".
// mode must be one of: "on", "true", "off", "false", "toggle".
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

// titleCase upper-cases the first rune of s. Suitable for known,
// controlled ASCII words (e.g. "welcome", "goodbye") — avoids pulling in
// golang.org/x/text/cases for a single-word capitalization.
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
	tag, _ := s.GetSetting(ctx.Ctx, kind+"_tag:"+chatKey)
	if tag == "" {
		tag = "on"
	}
	desc, _ := s.GetSetting(ctx.Ctx, kind+"_desc:"+chatKey)
	if desc == "" {
		desc = "off"
	}
	msgText, _ := s.GetSetting(ctx.Ctx, kind+"_msg:"+chatKey)
	if msgText == "" {
		msgText = "Default greeting text"
	}
	media, _ := s.GetSetting(ctx.Ctx, kind+"_media:"+chatKey)
	if media == "" {
		media = "None"
	}

	bodyText := fmt.Sprintf(`%s CONFIGURATION MENU

Group: %s
Status: %s
Tag Participant: %s
Include Group Description: %s
Media URL: %s
Custom Message: %s

Select an action below to toggle settings.`, strings.ToUpper(kind), groupName, strings.ToUpper(status), strings.ToUpper(tag), strings.ToUpper(desc), media, msgText)

	cmdPrefix := "." + kind
	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: new(bodyText),
					FooterText:  new("WhatsRook Group Greetings"),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons: []*waE2E.ButtonsMessage_Button{
						{
							ButtonID: new(cmdPrefix + " toggle"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("TOGGLE STATUS"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(cmdPrefix + " tag toggle"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("TOGGLE TAG"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(cmdPrefix + " desc toggle"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("TOGGLE DESC"),
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
