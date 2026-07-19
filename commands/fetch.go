package commands

import (
	"fmt"
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
	link := resolveFetchURL(ctx)
	if link == "" {
		return sendText(ctx, "_Usage: !fetch <url> (or reply to a message containing a url)_")
	}
	if !isSupportedFetchURL(link) {
		return sendText(ctx, "_Unsupported url. Supported: Instagram, TikTok, YouTube, Facebook, Threads, Twitter/X_")
	}

	var cookie string
	if isYouTubeURL(link) {
		cookie = getYouTubeCookie(ctx)
	}
	data, err := ember.Fetch(ctx.Ctx, link, cookie)
	if err != nil {
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
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
