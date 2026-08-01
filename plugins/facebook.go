// Facebook command – download media from Facebook URLs via Ember.
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
		Name:        "facebook",
		Aliases:     []string{"fb"},
		Description: "Download a Facebook video/reel",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleFacebook,
	})
}

func handleFacebook(ctx *Context) error {
	slog.Debug("handleFacebook started", "args", ctx.Args)
	if len(ctx.Args) == 0 {
		slog.Warn("handleFacebook: no URL provided")
		return sendText(ctx, "Usage: !facebook <url>")
	}
	if !utils.IsFacebookURL(ctx.Args[0]) {
		slog.Warn("handleFacebook: invalid URL", "url", ctx.Args[0])
		return sendText(ctx, "Invalid facebook url!")
	}
	loader := ctx.StartLoader("Downloading Facebook post")
	defer loader.Delete()

	slog.Debug("handleFacebook: calling Fetch", "url", ctx.Args[0])
	data, err := ember.Fetch(ctx.Ctx, ctx.Args[0], "")
	if err != nil {
		loader.Delete()
		slog.Error("handleFacebook: Fetch failed", "err", err)
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	slog.Debug("handleFacebook: Fetch success, calling SendResult")
	return sender.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}
