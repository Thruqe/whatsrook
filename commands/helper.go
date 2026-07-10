package commands

import "go.mau.fi/whatsmeow/proto/waE2E"

func sendText(ctx *Context, text string) error {
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		Conversation: new(text),
	})
	return err
}
