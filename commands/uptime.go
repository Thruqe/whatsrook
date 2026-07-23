// Uptime command – shows how long the daemon has been running.
package commands

import (
	"fmt"
	"time"
)

var startTime = time.Now()

func init() {
	Register(&Command{
		Name:        "uptime",
		Description: "Show how long the bot has been running",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleUptime,
	})
}

func handleUptime(ctx *Context) error {
	d := time.Since(startTime).Round(time.Second)
	return sendText(ctx, fmt.Sprintf("Uptime: %s", d))
}
