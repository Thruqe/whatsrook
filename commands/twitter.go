package commands

import (
	"fmt"

	"github.com/Thruqe/whatsrook/ember"
)

func init() {
	Register(&Command{
		Name:        "twitter",
		Aliases:     []string{"x"},
		Description: "Download a TikTok video",
		Handler:     handleTwitter,
	})
}

func handleTwitter(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return sendText(ctx, "Usage: !twitter <url>")
	}
	if !isTwitterURL(ctx.Args[0]) {
		return sendText(ctx, "_Invaild twitter url!_")
	}
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0])
	if err != nil {
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
