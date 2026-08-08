// NetPause command – checks network connection status and allows manual pause/resume.
package plugins

import (
	"fmt"
	"whatsrook/utils"
)

func init() {
	Register(&Command{
		Name:        "netpause",
		Aliases:     []string{"pause", "resume", "netstatus"},
		Description: "Check network health status or pause/resume process operations",
		Category:    "info",
		IsPublic:    false,
		Handler:     handleNetPause,
	})
}

func handleNetPause(ctx *Context) error {
	if len(ctx.Args) > 0 {
		arg := ctx.Args[0]
		switch arg {
		case "on", "pause", "true":
			utils.SetNetworkPaused(true, "manually paused by user command")
			return ctx.Reply("Process operations paused.")
		case "off", "resume", "false":
			utils.SetNetworkPaused(false, "")
			return ctx.Reply("Process operations resumed.")
		case "check":
			utils.CheckNetworkHealth()
		}
	}

	paused, reason, latency := utils.GetNetworkStatus()
	statusStr := "Running"
	if paused {
		statusStr = "Paused"
	}

	msg := fmt.Sprintf("Process Network Status: %s\nLatency: %v", statusStr, latency)
	if reason != "" {
		msg += fmt.Sprintf("\nReason: %s", reason)
	}
	return ctx.Reply(msg)
}
