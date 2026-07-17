package commands

import (
	"fmt"

	"github.com/Thruqe/whatsrook/ember"
)

func init() {
	Register(&Command{
		Name:        "tiktok",
		Aliases:     []string{"tt"},
		Description: "Download a TikTok video",
		Category:    "downloader",
		Handler:     handleTikTok,
	})
}

func handleTikTok(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return sendText(ctx, "Usage: !tiktok <url>")
	}
	if !isTikTokURL(ctx.Args[0]) {
		return sendText(ctx, "_Invaild tiktok url!_")
	}
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0])
	if err != nil {
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
