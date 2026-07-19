package commands

import (
	"fmt"
	"log/slog"

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
	slog.Info("handleInstagram started", "args", ctx.Args)
	if len(ctx.Args) == 0 {
		slog.Warn("handleInstagram: no URL provided")
		return sendText(ctx, "_Usage: !instagram <url>_")
	}
	if !isInstagramURL(ctx.Args[0]) {
		slog.Warn("handleInstagram: invalid URL", "url", ctx.Args[0])
		return sendText(ctx, "_Invaild instagram url!_")
	}
	slog.Info("handleInstagram: calling Fetch", "url", ctx.Args[0])
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0], "")
	if err != nil {
		slog.Error("handleInstagram: Fetch failed", "err", err)
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	slog.Info("handleInstagram: Fetch success, calling SendResult")
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
