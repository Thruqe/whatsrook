package commands

import (
	"fmt"
	"log/slog"

	"github.com/Thruqe/whatsrook/ember"
)

func init() {
	Register(&Command{
		Name:        "youtube",
		Aliases:     []string{"yt"},
		Description: "Download a YouTube video/short",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleYouTube,
	})
}

func handleYouTube(ctx *Context) error {
	slog.Info("handleYouTube started", "args", ctx.Args)
	if len(ctx.Args) == 0 {
		slog.Warn("handleYouTube: no URL provided")
		return sendText(ctx, "_Usage: !youtube <url>_")
	}
	if !isYouTubeURL(ctx.Args[0]) {
		slog.Warn("handleYouTube: invalid URL", "url", ctx.Args[0])
		return sendText(ctx, "_Invaild youtube url!_")
	}
	cookie := getYouTubeCookie(ctx)
	slog.Info("handleYouTube: calling Fetch", "url", ctx.Args[0], "cookie_len", len(cookie))
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0], cookie)
	if err != nil {
		slog.Error("handleYouTube: Fetch failed", "err", err)
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	slog.Info("handleYouTube: Fetch success, calling SendResult")
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
