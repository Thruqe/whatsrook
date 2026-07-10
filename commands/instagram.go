package commands

import (
	"fmt"

	"github.com/Thruqe/zevBot/ember"
)

func init() {
	Register(&Command{
		Name:        "instagram",
		Aliases:     []string{"ig"},
		Description: "Download an Instagram reel/post",
		Handler:     handleInstagram,
	})
}

func handleInstagram(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return sendText(ctx, "Usage: !instagram <url>")
	}
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0])
	if err != nil {
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
