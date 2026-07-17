package commands

import (
	"fmt"

	"github.com/Thruqe/whatsrook/ember"
)

func init() {
	Register(&Command{
		Name:        "threads",
		Aliases:     []string{"th"},
		Description: "Download a Threads post",
		Category:    "downloader",
		Handler:     handleThreads,
	})
}

func handleThreads(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return sendText(ctx, "Usage: !threads <url>")
	}
	if !isThreadsURL(ctx.Args[0]) {
		return sendText(ctx, "_Invaild threads url!_")
	}
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0])
	if err != nil {
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
