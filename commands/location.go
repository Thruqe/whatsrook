package commands

import (
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func init() {
	Register(&Command{
		Name:        "locbuttons",
		Description: "Send an interactive message with a location header and buttons",
		Category:    "example",
		IsPublic:    true,
		Handler:     handleLocButtons,
	})
}

func handleLocButtons(ctx *Context) error {
	msgVersion := int32(1)

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: &waE2E.InteractiveMessage{
					Header: &waE2E.InteractiveMessage_Header{
						HasMediaAttachment: proto.Bool(true),
						Media: &waE2E.InteractiveMessage_Header_LocationMessage{
							LocationMessage: &waE2E.LocationMessage{
								DegreesLatitude:  proto.Float64(37.4849),
								DegreesLongitude: proto.Float64(-122.1484),
								Name:             proto.String("Meta Headquarters"),
								Address:          proto.String("1 Hacker Way, Menlo Park, CA 94025"),
							},
						},
					},
					Body: &waE2E.InteractiveMessage_Body{
						Text: proto.String("📍 *Location Details*\n\nHere is our primary office location. Use the actions below for navigation."),
					},
					Footer: &waE2E.InteractiveMessage_Footer{
						Text: proto.String("Powered by Thruqe"),
					},
					InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
						NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
							Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
								{
									Name:             proto.String("cta_url"),
									ButtonParamsJSON: proto.String(`{"display_text":"🗺️ Open in Google Maps","url":"https://maps.google.com/?q=37.4849,-122.1484","merchant_url":"https://maps.google.com/?q=37.4849,-122.1484"}`),
								},
								{
									Name:             proto.String("quick_reply"),
									ButtonParamsJSON: proto.String(`{"display_text":"🏠 Return Main","id":"return_main"}`),
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
