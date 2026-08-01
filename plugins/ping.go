// Ping command – replies with latency and message ID timing info.
package commands

import (
	"fmt"
	"time"
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

	loader := ctx.StartLoader("Ponging")
	defer loader.Stop()

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

	_, err := ctx.Edit(loader.MessageID(), text)
	return err
}
