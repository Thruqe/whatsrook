package commands

import (
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

func init() {
	Register(&Command{
		Name:        "ping",
		Description: "Check if the bot is alive and measure response latency",
		Category:    "info",
		IsPublic:    true,
		Handler:     handlePing,
	})
}

func handlePing(ctx *Context) error {
	start := time.Now()

	pongText := new("Pong...")

	resp, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		Conversation: pongText,
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start)
	_ = elapsed

	elapsedText := new("Pong! " + elapsed.String())

	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, ctx.Client.BuildEdit(ctx.Chat, resp.ID, &waE2E.Message{
		Conversation: elapsedText,
	}))
	if err != nil {
		return err
	}

	return nil
}
