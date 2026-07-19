package commands

import (
	"fmt"
	"log/slog"

	"github.com/Thruqe/whatsrook/ember"
)

func init() {
	Register(&Command{
		Name:        "facebook",
		Aliases:     []string{"fb"},
		Description: "Download a Facebook video/reel",
		Category:    "downloader",
		IsPublic:     true,
		Handler:     handleFacebook,
	})
}

func handleFacebook(ctx *Context) error {
	slog.Info("handleFacebook started", "args", ctx.Args)
	if len(ctx.Args) == 0 {
		slog.Warn("handleFacebook: no URL provided")
		return sendText(ctx, "_Usage: !facebook <url>_")
	}
	if !isFacebookURL(ctx.Args[0]) {
		slog.Warn("handleFacebook: invalid URL", "url", ctx.Args[0])
		return sendText(ctx, "_Invaild facebook url!_")
	}
	slog.Info("handleFacebook: calling Fetch", "url", ctx.Args[0])
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0], "")
	if err != nil {
		slog.Error("handleFacebook: Fetch failed", "err", err)
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	slog.Info("handleFacebook: Fetch success, calling SendResult")
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
