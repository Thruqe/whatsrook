package commands

import (
	"fmt"
	"log/slog"

	"whatsrook/ember"
	"whatsrook/sender"
	"whatsrook/utils"
)

func init() {
	Register(&Command{
		Name:        "twitter",
		Aliases:     []string{"x", "twt"},
		Description: "Download a Twitter/X video/media",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleTwitter,
	})
}

func handleTwitter(ctx *Context) error {
	slog.Info("handleTwitter started", "args", ctx.Args)
	if len(ctx.Args) == 0 {
		slog.Warn("handleTwitter: no URL provided")
		return sendText(ctx, "Usage: !twitter <url>")
	}
	if !utils.IsTwitterURL(ctx.Args[0]) {
		slog.Warn("handleTwitter: invalid URL", "url", ctx.Args[0])
		return sendText(ctx, "Invalid twitter/x url!")
	}
	slog.Info("handleTwitter: calling Fetch", "url", ctx.Args[0])
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0], "")
	if err != nil {
		slog.Error("handleTwitter: Fetch failed", "err", err)
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	slog.Info("handleTwitter: Fetch success, calling SendResult")
	return sender.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
