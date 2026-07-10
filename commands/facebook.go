package commands

import (
	"fmt"

	"github.com/Thruqe/zevBot/ember"
)

func init() {
	Register(&Command{
		Name:        "facebook",
		Aliases:     []string{"fb"},
		Description: "Download a Facebook video/reel",
		Handler:     handleFacebook,
	})
}

func handleFacebook(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return sendText(ctx, "Usage: !facebook <url>")
	}
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0])
	if err != nil {
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
