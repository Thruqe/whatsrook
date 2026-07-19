package commands

import (
	"fmt"
	"log/slog"

	"github.com/Thruqe/whatsrook/ember"
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
		return sendText(ctx, "_Usage: !twitter <url>_")
	}
	if !isTwitterURL(ctx.Args[0]) {
		slog.Warn("handleTwitter: invalid URL", "url", ctx.Args[0])
		return sendText(ctx, "_Invaild twitter/x url!_")
	}
	slog.Info("handleTwitter: calling Fetch", "url", ctx.Args[0])
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0], "")
	if err != nil {
		slog.Error("handleTwitter: Fetch failed", "err", err)
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	slog.Info("handleTwitter: Fetch success, calling SendResult")
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
