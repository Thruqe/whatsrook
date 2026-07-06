package commands

import (
	"fmt"

	"github.com/Thruqe/zevBot/ember"
)

func init() {
	Register(&Command{
		Name:        "threads",
		Aliases:     []string{"th"},
		Description: "Download a Threads post",
		Handler:     handleThreads,
	})
}

func handleThreads(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return sendText(ctx, "Usage: !threads <url>")
	}
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0])
	if err != nil {
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
