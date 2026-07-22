package commands

import (
	"fmt"
	"log/slog"

	"github.com/Thruqe/whatsrook/ember"
	"github.com/Thruqe/whatsrook/sender"
	"github.com/Thruqe/whatsrook/utils"
)

func init() {
	Register(&Command{
		Name:        "threads",
		Aliases:     []string{"th"},
		Description: "Download a Threads post",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleThreads,
	})
}

func handleThreads(ctx *Context) error {
	slog.Info("handleThreads started", "args", ctx.Args)
	if len(ctx.Args) == 0 {
		slog.Warn("handleThreads: no URL provided")
		return sendText(ctx, "Usage: !threads <url>")
	}
	if !utils.IsThreadsURL(ctx.Args[0]) {
		slog.Warn("handleThreads: invalid URL", "url", ctx.Args[0])
		return sendText(ctx, "Invalid threads url!")
	}
	slog.Info("handleThreads: calling Fetch", "url", ctx.Args[0])
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0], "")
	if err != nil {
		slog.Error("handleThreads: Fetch failed", "err", err)
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	slog.Info("handleThreads: Fetch success, calling SendResult")
	return sender.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
