// Repo command – displays the repository link and star request with interactive buttons.
package plugins

import (
	"whatsrook/send"

	"whatsrook/wa-core"
	waBinary "whatsrook/wa-core/binary"
	"whatsrook/wa-core/proto/waE2E"
)

func init() {
	Register(&Command{
		Name:        "repo",
		Aliases:     []string{"sc", "script", "github"},
		Description: "Show the GitHub repository link and project info",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleRepo,
	})
}

func handleRepo(ctx *Context) error {
	repoURL := "https://github.com/Thruqe/whatsrook"
	bodyText := "WhatsRook Repository\n\nGitHub: " + repoURL + "\n\nPlease star the repository if you like the project, it helps support and motivate me."

	cmdPrefix := ctx.GetPrefix()
	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: new(bodyText),
					FooterText:  new("WhatsRook Open Source Project"),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons: []*waE2E.ButtonsMessage_Button{
						{
							ButtonID: new(cmdPrefix + "menu"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("MENU"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(cmdPrefix + "ping"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("PING"),
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
	if err != nil {
		return ctx.Reply(send.FormatTextResponseRaw(bodyText))
	}
	return nil
}
