package commands

import (
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func init() {
	Register(&Command{
		Name:        "buttons",
		Description: "Send an interactive message with action buttons",
		Category:    "interactive",
		IsPublic:    true,
		Handler:     handleButtons,
	})
}

func handleButtons(ctx *Context) error {
	msgVersion := int32(1)

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: &waE2E.InteractiveMessage{
					Body: &waE2E.InteractiveMessage_Body{
						Text: new("Interactive Buttons\n\nChoose an action below to proceed."),
					},
					Footer: &waE2E.InteractiveMessage_Footer{
						Text: new("Powered by Thruqe"),
					},
					InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
						NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
							Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
								{
									Name:             new("quick_reply"),
									ButtonParamsJSON: new(`{"display_text":"Say Hello","id":"hello_reply"}`),
								},
								{
									Name:             new("cta_url"),
									ButtonParamsJSON: new(`{"display_text":"Visit Website","url":"https://github.com/Thruqe/whatsrook","merchant_url":"https://github.com/Thruqe/whatsrook"}`),
								},
								{
									Name:             new("cta_call"),
									ButtonParamsJSON: new(`{"display_text":"Call Support","phone_number":"+1234567890"}`),
								},
								{
									Name:             new("cta_copy"),
									ButtonParamsJSON: new(`{"display_text":"Copy Command","id":"copy_cmd","copy_code":".ping"}`),
								},
							},
							MessageVersion: &msgVersion,
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
