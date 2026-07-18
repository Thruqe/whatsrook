package commands

import (
	"fmt"

	"github.com/Thruqe/whatsrook/ember"
)

func init() {
	Register(&Command{
		Name:        "youtube",
		Aliases:     []string{"yt"},
		Description: "Download a YouTube video/short",
		Category:    "downloader",
		IsPublic:     true,
		Handler:     handleYouTube,
	})
}

func handleYouTube(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return sendText(ctx, "_Usage: !youtube <url>_")
	}
	if !isYouTubeURL(ctx.Args[0]) {
		return sendText(ctx, "_Invaild youtube url!_")
	}
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0])
	if err != nil {
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
