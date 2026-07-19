package commands

import (
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
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

	resp, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		Conversation: proto.String("🏓 Pong..."),
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start)

	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, ctx.Client.BuildEdit(ctx.Chat, resp.ID, &waE2E.Message{
		Conversation: proto.String("🏓 Pong! " + elapsed.String()),
	}))
	if err != nil {
		return err
	}

	return nil
}
