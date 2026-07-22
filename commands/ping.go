package commands

import (
	"fmt"
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

	pongText := new("🏓 Ponging...")

	resp, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		Conversation: pongText,
	})
	if err != nil {
		return err
	}

	elapsed := time.Since(start)

	var latency string
	if elapsed < time.Millisecond {
		latency = fmt.Sprintf("%.2f µs", float64(elapsed.Microseconds()))
	} else if elapsed < time.Second {
		latency = fmt.Sprintf("%d ms", elapsed.Milliseconds())
	} else {
		latency = fmt.Sprintf("%.2f s", elapsed.Seconds())
	}

	text := fmt.Sprintf("Pong!\nResponse Time: %s", latency)
	if ctx.Evt != nil && !ctx.Evt.Info.Timestamp.IsZero() {
		msgLag := start.Sub(ctx.Evt.Info.Timestamp)
		if msgLag > 0 {
			text += fmt.Sprintf("\nIncoming Lag: %d ms", msgLag.Milliseconds())
		}
	}

	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, ctx.Client.BuildEdit(ctx.Chat, resp.ID, &waE2E.Message{
		Conversation: &text,
	}))
	return err
}
