package commands

import (
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func init() {
	Register(&Command{
		Name:        "statusmenu",
		Description: "Send a status menu with buttons and location header",
		Category:    "interactive",
		IsPublic:    true,
		Handler:     handleStatusMenu,
	})
}

func handleStatusMenu(ctx *Context) error {
	bodyText := `┌─ム ᴛʜʀᴜQᴇ ᴍᴜʟᴛɪᴅᴇᴠɪᴄᴇ
│ ᴏᴡɴᴇʀ: ᴛʜʀᴜQᴇ
│ ᴜsᴇʀ: thruqe
│ ᴄᴀᴛᴇɢᴏʀɪᴇs: 10
│ ᴄᴏᴍᴍᴀɴᴅs: 271
│ sᴘᴇᴇᴅ: 2025.91 ᴍs
│ ᴜᴘᴛɪᴍᴇ: 9633 s
╰──────────────────╯

┌─ム ʙᴏᴛ sᴛᴀᴛᴜs
│
├─ム  ʙᴏᴛ: ᴏɴʟɪɴᴇ
├─ム  sᴛᴀᴛᴜs: ʀᴇᴀᴅʏ
├─ム  sᴇʟᴇᴄᴛ ᴀ ʙᴜᴛᴛᴏɴ ʙᴇʟᴏᴡ
│
╰─────────◆────────╯

> 「 Powered by Thruqe 」`

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					Header: &waE2E.ButtonsMessage_LocationMessage{
						LocationMessage: &waE2E.LocationMessage{
							DegreesLatitude:  proto.Float64(0),
							DegreesLongitude: proto.Float64(0),
							Name:             new("Thruqe"),
							Address:          new("Thruqe Multidevice"),
						},
					},
					ContentText: new(bodyText),
					FooterText:  new("「 Powered by Thruqe 」"),
					HeaderType:  waE2E.ButtonsMessage_LOCATION.Enum(),
					Buttons: []*waE2E.ButtonsMessage_Button{
						{
							ButtonID: new("menu-btn"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("MENU"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new("menu all"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("COMMANDS"),
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

	// 1. Send the status message
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	if err != nil {
		return err
	}

	// 2. React to the trigger message
	reactionMsg := ctx.Client.BuildReaction(ctx.Chat, ctx.Sender, ctx.Evt.Info.ID, "")
	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, reactionMsg)
	return err
}
