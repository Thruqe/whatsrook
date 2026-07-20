package commands

import (
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func init() {
	Register(&Command{
		Name:        "template",
		Description: "Send an interactive legacy template message",
		Category:    "example",
		IsPublic:    true,
		Handler:     handleTemplate,
	})
}

func handleTemplate(ctx *Context) error {
	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				TemplateMessage: &waE2E.TemplateMessage{
					HydratedTemplate: &waE2E.TemplateMessage_HydratedFourRowTemplate{
						HydratedContentText: proto.String("📝 *Template Message Demo*\n\nThis is a legacy template message format containing call-to-action buttons."),
						HydratedFooterText:  proto.String("Powered by Thruqe"),
						HydratedButtons: []*waE2E.HydratedTemplateButton{
							{
								Index: proto.Uint32(1),
								HydratedButton: &waE2E.HydratedTemplateButton_QuickReplyButton{
									QuickReplyButton: &waE2E.HydratedTemplateButton_HydratedQuickReplyButton{
										DisplayText: proto.String("👋 Quick Reply"),
										ID:          proto.String("temp_qr"),
									},
								},
							},
							{
								Index: proto.Uint32(2),
								HydratedButton: &waE2E.HydratedTemplateButton_UrlButton{
									UrlButton: &waE2E.HydratedTemplateButton_HydratedURLButton{
										DisplayText: proto.String("🌐 Open Link"),
										URL:         proto.String("https://github.com/Thruqe/whatsrook"),
									},
								},
							},
							{
								Index: proto.Uint32(3),
								HydratedButton: &waE2E.HydratedTemplateButton_CallButton{
									CallButton: &waE2E.HydratedTemplateButton_HydratedCallButton{
										DisplayText: proto.String("📞 Call Us"),
										PhoneNumber: proto.String("+1234567890"),
									},
								},
							},
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
					"type": "template",
					"v":    "1",
				},
				Content: []waBinary.Node{
					{
						Tag: "template",
						Attrs: waBinary.Attrs{
							"v": "1",
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
