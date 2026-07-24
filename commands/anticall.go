// AntiCall command – configure call rejection rules, contact filters, country code filters, and spam warning thresholds.
package commands

import (
	"fmt"
	"strconv"
	"strings"

	"whatsrook/store/sqlstore"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
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

	case "contacts":
		if len(args) < 2 {
			curr, _ := s.GetSetting(ctx.Ctx, "anticall_contacts_only")
			return ctx.Reply("AntiCall contacts only setting: " + curr)
		}
		mode := strings.ToLower(args[1])
		if mode == "on" || mode == "true" {
			_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "true")
			return ctx.Reply("AntiCall set to allow calls from contacts only.")
		} else if mode == "off" || mode == "false" {
			_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "false")
			return ctx.Reply("AntiCall contacts only restriction disabled.")
		} else if mode == "toggle" {
			curr, _ := s.GetSetting(ctx.Ctx, "anticall_contacts_only")
			if curr == "true" {
				_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "false")
				return ctx.Reply("AntiCall contacts only restriction disabled.")
			}
			_ = s.PutSetting(ctx.Ctx, "anticall_contacts_only", "true")
			return ctx.Reply("AntiCall set to allow calls from contacts only.")
		}
		return ctx.Reply("Usage: .anticall contacts [on|off|toggle]")

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
				return ctx.Reply("Usage: .anticall cc add <country_code>")
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
				return ctx.Reply("Usage: .anticall cc del <country_code>")
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
			return ctx.Reply("Usage: .anticall cc [add|del|clear]")
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
		return ctx.Reply("Usage: .anticall [on|off|toggle|contacts|cc|warn]")
	}
}

func sendAntiCallMenu(ctx *Context, s *sqlstore.SQLStore) error {
	status, _ := s.GetSetting(ctx.Ctx, "anticall_status")
	if status == "" {
		status = "off"
	}
	contactsOnly, _ := s.GetSetting(ctx.Ctx, "anticall_contacts_only")
	if contactsOnly == "" {
		contactsOnly = "false"
	}
	allowedCC, _ := s.GetSetting(ctx.Ctx, "anticall_allowed_cc")
	if allowedCC == "" {
		allowedCC = "all"
	}
	maxWarn, _ := s.GetSetting(ctx.Ctx, "anticall_max_warn")
	if maxWarn == "" {
		maxWarn = "3"
	}

	bodyText := fmt.Sprintf(`ANTICALL SETTINGS MENU

Status: %s
Contacts Only: %s
Allowed Country Codes: %s
Max Warnings Before Block: %s

Select an option below to change settings.`, strings.ToUpper(status), strings.ToUpper(contactsOnly), allowedCC, maxWarn)

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					Header: &waE2E.ButtonsMessage_LocationMessage{
						LocationMessage: &waE2E.LocationMessage{
							DegreesLatitude:  proto.Float64(0),
							DegreesLongitude: proto.Float64(0),
							Name:             new("AntiCall Configuration"),
							Address:          new("WhatsRook Security"),
						},
					},
					ContentText: new(bodyText),
					FooterText:  new("WhatsRook AntiCall Settings"),
					HeaderType:  waE2E.ButtonsMessage_LOCATION.Enum(),
					Buttons: []*waE2E.ButtonsMessage_Button{
						{
							ButtonID: new(".anticall toggle"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("TOGGLE STATUS"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(".anticall contacts toggle"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("TOGGLE CONTACTS ONLY"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(".anticall warn 3"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("RESET WARN THRESHOLD"),
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
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
