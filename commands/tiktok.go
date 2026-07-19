package commands

import (
	"fmt"
	"log/slog"

	"github.com/Thruqe/whatsrook/ember"
)

func init() {
	Register(&Command{
		Name:        "tiktok",
		Aliases:     []string{"tt"},
		Description: "Download a TikTok video",
		Category:    "downloader",
		IsPublic:     true,
		Handler:     handleTikTok,
	})
}

func handleTikTok(ctx *Context) error {
	slog.Info("handleTikTok started", "args", ctx.Args)
	if len(ctx.Args) == 0 {
		slog.Warn("handleTikTok: no URL provided")
		return sendText(ctx, "Usage: !tiktok <url>")
	}
	if !isTikTokURL(ctx.Args[0]) {
		slog.Warn("handleTikTok: invalid URL", "url", ctx.Args[0])
		return sendText(ctx, "_Invaild tiktok url!_")
	}
	slog.Info("handleTikTok: calling Fetch", "url", ctx.Args[0])
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0], "")
	if err != nil {
		slog.Error("handleTikTok: Fetch failed", "err", err)
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	slog.Info("handleTikTok: Fetch success, calling SendResult")
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
