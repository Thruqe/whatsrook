// Ping command – replies with latency and message ID timing info.
package plugins

import (
	"fmt"
	"strings"
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

	msgID, err := ctx.ReplyWithID("⚡ Ping...")
	if err != nil {
		return err
	}

	end := time.Now()
	elapsed := end.Sub(start)

	var latency string
	if elapsed < time.Millisecond {
		latency = fmt.Sprintf("%.2f µs", float64(elapsed.Microseconds()))
	} else if elapsed < time.Second {
		latency = fmt.Sprintf("%d ms", elapsed.Milliseconds())
	} else {
		latency = fmt.Sprintf("%.2f s", elapsed.Seconds())
	}

	startTimeStr := start.Format("15:04:05.000")
	endTimeStr := end.Format("15:04:05.000")

	var sb strings.Builder
	sb.WriteString("🏓 *Pong!*\n\n")
	fmt.Fprintf(&sb, "• Start  : `%s`\n", startTimeStr)
	fmt.Fprintf(&sb, "• End    : `%s`\n", endTimeStr)
	fmt.Fprintf(&sb, "• Latency: `%s`", latency)

	if ctx.Evt != nil && !ctx.Evt.Info.Timestamp.IsZero() {
		msgLag := start.Sub(ctx.Evt.Info.Timestamp)
		if msgLag > 0 {
			fmt.Fprintf(&sb, "\n• Lag    : `%d ms`", msgLag.Milliseconds())
		}
	}

	_, editErr := ctx.Edit(msgID, sb.String())
	if editErr != nil {
		_ = ctx.Reply(sb.String())
	}
	return nil
}
