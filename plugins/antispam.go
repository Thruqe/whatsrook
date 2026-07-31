// AntiSpam command – configure group anti-spam rules, message rate limits, and automated actions.
package commands

import (
	"fmt"
	"strconv"
	"strings"

	"whatsrook/store/sqlstore"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
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
		return ctx.Reply("Usage: .antispam [on|off|toggle|action|max]")
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
	action, _ := s.GetSetting(ctx.Ctx, "antispam_action:"+chatKey)
	if action == "" {
		action = "delete"
	}
	maxVal, _ := s.GetSetting(ctx.Ctx, "antispam_max:"+chatKey)
	if maxVal == "" {
		maxVal = "5"
	}

	bodyText := fmt.Sprintf(`ANTISPAM CONFIGURATION MENU

Group: %s
Status: %s
Action: %s
Max Messages (per 5s): %s

Select an action below to toggle settings.`, groupName, strings.ToUpper(status), strings.ToUpper(action), maxVal)

	cmdPrefix := ctx.GetPrefix()
	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: new(bodyText),
					FooterText:  new(fmt.Sprintf("%s AntiSpam Moderation", ctx.GetBotName())),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons: []*waE2E.ButtonsMessage_Button{
						{
							ButtonID: new(cmdPrefix + "antispam toggle"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("TOGGLE STATUS"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(cmdPrefix + "antispam action delete"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("ACTION DELETE"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(cmdPrefix + "antispam max 5"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("RESET LIMIT (5)"),
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
