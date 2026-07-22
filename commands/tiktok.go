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
		Name:        "tiktok",
		Aliases:     []string{"tt"},
		Description: "Download a TikTok video",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleTikTok,
	})
}

func handleTikTok(ctx *Context) error {
	slog.Info("handleTikTok started", "args", ctx.Args)
	if len(ctx.Args) == 0 {
		slog.Warn("handleTikTok: no URL provided")
		return sendText(ctx, "Usage: !tiktok <url>")
	}
	if !utils.IsTikTokURL(ctx.Args[0]) {
		slog.Warn("handleTikTok: invalid URL", "url", ctx.Args[0])
		return sendText(ctx, "Invalid tiktok url!")
	}
	slog.Info("handleTikTok: calling Fetch", "url", ctx.Args[0])
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0], "")
	if err != nil {
		slog.Error("handleTikTok: Fetch failed", "err", err)
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	slog.Info("handleTikTok: Fetch success, calling SendResult")
	return sender.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
