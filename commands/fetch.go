package commands

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/Thruqe/whatsrook/ember"
)

var urlPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

func init() {
	Register(&Command{
		Name:        "fetch",
		Aliases:     []string{"dl", "download"},
		Description: "Download media from Instagram, TikTok, YouTube, Facebook, Threads, or Twitter/X",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleFetch,
	})
}

func handleFetch(ctx *Context) error {
	slog.Info("handleFetch started", "raw_args", ctx.RawArgs)
	link := resolveFetchURL(ctx)
	if link == "" {
		slog.Warn("handleFetch: no URL resolved")
		return sendText(ctx, "_Usage: !fetch <url> (or reply to a message containing a url)_")
	}
	slog.Info("handleFetch: resolved URL", "url", link)
	if !isSupportedFetchURL(link) {
		slog.Warn("handleFetch: URL is unsupported", "url", link)
		return sendText(ctx, "_Unsupported url. Supported: Instagram, TikTok, YouTube, Facebook, Threads, Twitter/X_")
	}

	var cookie string
	if isYouTubeURL(link) {
		cookie = getYouTubeCookie(ctx)
		slog.Info("handleFetch: YouTube cookie retrieved", "cookie_len", len(cookie))
	}
	slog.Info("handleFetch: calling ember.Fetch", "url", link)
	data, err := ember.Fetch(ctx.Ctx, link, cookie)
	if err != nil {
		slog.Error("handleFetch: ember.Fetch failed", "err", err)
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	slog.Info("handleFetch: ember.Fetch success, calling SendResult", "title", data.Title, "medias_count", len(data.Medias))
	return ember.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}

// resolveFetchURL picks a media URL from command args, otherwise from a quoted message.
func resolveFetchURL(ctx *Context) string {
	if ctx.RawArgs != "" {
		if link := firstURL(ctx.RawArgs); link != "" {
			return link
		}
		// Allow bare args that are already a single URL-like token.
		if len(ctx.Args) > 0 {
			return strings.TrimSpace(ctx.Args[0])
		}
	}

	if quoted := ctx.GetQuotedMessage(); quoted != nil {
		if text := extractTextFromProto(quoted); text != "" {
			if link := firstURL(text); link != "" {
				return link
			}
		}
	}

	return ""
}

func firstURL(text string) string {
	m := urlPattern.FindString(text)
	return strings.TrimRight(m, ".,);]!?")
}

func isSupportedFetchURL(link string) bool {
	return isInstagramURL(link) ||
		isTikTokURL(link) ||
		isYouTubeURL(link) ||
		isFacebookURL(link) ||
		isThreadsURL(link) ||
		isTwitterURL(link)
}
