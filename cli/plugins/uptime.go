// Uptime command – shows how long the daemon has been running.
package plugins

import (
	"fmt"
	"strings"
	"time"
)

var startTime = time.Now()

func init() {
	Register(&Command{
		Name:        "uptime",
		Aliases:     []string{"runtime"},
		Description: "Show how long the bot has been running",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleUptime,
	})
}

func handleUptime(ctx *Context) error {
	d := time.Since(startTime).Round(time.Second)

	totalSeconds := int(d.Seconds())
	days := totalSeconds / 86400
	hours := (totalSeconds % 86400) / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}

	out := strings.Join(parts, " ")
	return sendText(ctx, out)
}
