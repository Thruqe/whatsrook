package commands

import (
	"fmt"

	"github.com/Thruqe/zevBot/ember"
)

func init() {
	Register(&Command{
		Name:        "youtube",
		Aliases:     []string{"yt"},
		Description: "Download a YouTube video/short",
		Handler:     handleYouTube,
	})
}

func handleYouTube(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return sendText(ctx, "Usage: !youtube <url>")
	}
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0])
	if err != nil {
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
