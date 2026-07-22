package commands

import (
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func init() {
	Register(&Command{
		Name:        "selectlist",
		Description: "Send a dropdown list/menu of options",
		Category:    "interactive",
		IsPublic:    true,
		Handler:     handleSelectList,
	})
}

func handleSelectList(ctx *Context) error {
	msgVersion := int32(1)

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: &waE2E.InteractiveMessage{
					Body: &waE2E.InteractiveMessage_Body{
						Text: new("Select an Option\n\nClick the button below to view the available items."),
					},
					Footer: &waE2E.InteractiveMessage_Footer{
						Text: new("Powered by Thruqe"),
					},
					InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
						NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
							Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
								{
									Name: new("single_select"),
									ButtonParamsJSON: new(`{
										"title": "View List Menu",
										"sections": [
											{
												"title": "Interactive Demos",
												"rows": [
													{
														"id": "demo_buttons",
														"title": "Action Buttons",
														"description": "Send a demo of buttons"
													},
													{
														"id": "demo_gallery",
														"title": "Image Carousel Gallery",
														"description": "Send a demo of image carousel"
													}
												]
											},
											{
												"title": "System Commands",
												"rows": [
													{
														"id": "cmd_ping",
														"title": "Ping Latency Check",
														"description": "Measure connection speed"
													},
													{
														"id": "cmd_sysinfo",
														"title": "System Status",
														"description": "Show cpu and memory info"
													}
												]
											}
										]
									}`),
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
