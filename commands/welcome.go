// Welcome and Goodbye commands – configure group join/leave greetings with buttons, tags, group descriptions, custom text templates, and media.
package commands

import (
	"fmt"
	"strings"

	"whatsrook/store/sqlstore"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

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
	statusKey := kind + "_status:" + chatKey
	tagKey := kind + "_tag:" + chatKey
	descKey := kind + "_desc:" + chatKey
	msgKey := kind + "_msg:" + chatKey
	mediaKey := kind + "_media:" + chatKey

	args := strings.Fields(ctx.RawArgs)
	if len(args) == 0 {
		return sendGreetingMenu(ctx, s, kind)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "on", "enable":
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply(strings.Title(kind) + " message enabled for this group.")

	case "off", "disable":
		_ = s.PutSetting(ctx.Ctx, statusKey, "off")
		return ctx.Reply(strings.Title(kind) + " message disabled for this group.")

	case "toggle":
		curr, _ := s.GetSetting(ctx.Ctx, statusKey)
		if curr == "on" {
			_ = s.PutSetting(ctx.Ctx, statusKey, "off")
			return ctx.Reply(strings.Title(kind) + " message disabled for this group.")
		}
		_ = s.PutSetting(ctx.Ctx, statusKey, "on")
		return ctx.Reply(strings.Title(kind) + " message enabled for this group.")

	case "tag":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, tagKey)
			return ctx.Reply(strings.Title(kind) + " participant tag setting: " + curr)
		}
		mode := strings.ToLower(args[1])
		if mode == "on" || mode == "true" {
			_ = s.PutSetting(ctx.Ctx, tagKey, "on")
			return ctx.Reply(strings.Title(kind) + " participant tagging enabled.")
		} else if mode == "off" || mode == "false" {
			_ = s.PutSetting(ctx.Ctx, tagKey, "off")
			return ctx.Reply(strings.Title(kind) + " participant tagging disabled.")
		} else if mode == "toggle" {
			curr, _ := s.GetSetting(ctx.Ctx, tagKey)
			if curr == "on" {
				_ = s.PutSetting(ctx.Ctx, tagKey, "off")
				return ctx.Reply(strings.Title(kind) + " participant tagging disabled.")
			}
			_ = s.PutSetting(ctx.Ctx, tagKey, "on")
			return ctx.Reply(strings.Title(kind) + " participant tagging enabled.")
		}
		return ctx.Reply("Usage: ." + kind + " tag [on|off|toggle]")

	case "desc":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, descKey)
			return ctx.Reply(strings.Title(kind) + " group description setting: " + curr)
		}
		mode := strings.ToLower(args[1])
		if mode == "on" || mode == "true" {
			_ = s.PutSetting(ctx.Ctx, descKey, "on")
			return ctx.Reply(strings.Title(kind) + " group description inclusion enabled.")
		} else if mode == "off" || mode == "false" {
			_ = s.PutSetting(ctx.Ctx, descKey, "off")
			return ctx.Reply(strings.Title(kind) + " group description inclusion disabled.")
		} else if mode == "toggle" {
			curr, _ := s.GetSetting(ctx.Ctx, descKey)
			if curr == "on" {
				_ = s.PutSetting(ctx.Ctx, descKey, "off")
				return ctx.Reply(strings.Title(kind) + " group description inclusion disabled.")
			}
			_ = s.PutSetting(ctx.Ctx, descKey, "on")
			return ctx.Reply(strings.Title(kind) + " group description inclusion enabled.")
		}
		return ctx.Reply("Usage: ." + kind + " desc [on|off|toggle]")

	case "msg", "message", "text":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, msgKey)
			if curr == "" {
				curr = "none (using default template)"
			}
			return ctx.Reply(strings.Title(kind) + " custom message template: " + curr)
		}
		text := strings.TrimSpace(strings.TrimPrefix(ctx.RawArgs, args[0]))
		_ = s.PutSetting(ctx.Ctx, msgKey, text)
		return ctx.Reply(strings.Title(kind) + " custom message template updated.")

	case "media", "video":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, mediaKey)
			if curr == "" {
				curr = "none"
			}
			return ctx.Reply(strings.Title(kind) + " media URL: " + curr)
		}
		url := strings.TrimSpace(args[1])
		if url == "none" || url == "clear" {
			_ = s.PutSetting(ctx.Ctx, mediaKey, "")
			return ctx.Reply(strings.Title(kind) + " media cleared.")
		}
		_ = s.PutSetting(ctx.Ctx, mediaKey, url)
		return ctx.Reply(strings.Title(kind) + " media URL saved.")

	default:
		return ctx.Reply("Usage: ." + kind + " [on|off|toggle|tag|desc|msg|media]")
	}
}

func sendGreetingMenu(ctx *Context, s *sqlstore.SQLStore, kind string) error {
	chatKey := ctx.Chat.String()
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

Select an action below to toggle settings.`, strings.ToUpper(kind), chatKey, strings.ToUpper(status), strings.ToUpper(tag), strings.ToUpper(desc), media, msgText)

	cmdPrefix := "." + kind
	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					Header: &waE2E.ButtonsMessage_LocationMessage{
						LocationMessage: &waE2E.LocationMessage{
							DegreesLatitude:  proto.Float64(0),
							DegreesLongitude: proto.Float64(0),
							Name:             new(strings.Title(kind) + " Settings"),
							Address:          new("WhatsRook Group Greetings"),
						},
					},
					ContentText: new(bodyText),
					FooterText:  new("WhatsRook Group Greetings"),
					HeaderType:  waE2E.ButtonsMessage_LOCATION.Enum(),
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
