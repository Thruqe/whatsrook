package commands

import (
	"fmt"

	"github.com/Thruqe/whatsrook/ember"
)

func init() {
	Register(&Command{
		Name:        "instagram",
		Aliases:     []string{"ig"},
		Description: "Download an Instagram reel/post",
		Category:    "downloader",
		IsPublic:     true,
		Handler:     handleInstagram,
	})
}

func handleInstagram(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return sendText(ctx, "_Usage: !instagram <url>_")
	}
	if !isInstagramURL(ctx.Args[0]) {
		return sendText(ctx, "_Invaild instagram url!_")
	}
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0], "")
	if err != nil {
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
