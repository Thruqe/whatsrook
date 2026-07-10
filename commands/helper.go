package commands

import (
	"net/url"
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

func sendText(ctx *Context, text string) error {
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		Conversation: new(text),
	})
	return err
}

func isFacebookURL(link string) bool {
	return matchesHost(link, "facebook.com", "fb.com", "fb.watch")
}

func isInstagramURL(link string) bool {
	return matchesHost(link, "instagram.com")
}

func isTwitterURL(link string) bool {
	return matchesHost(link, "twitter.com", "x.com")
}

func isThreadsURL(link string) bool {
	return matchesHost(link, "threads.net")
}

func isYouTubeURL(link string) bool {
	return matchesHost(link, "youtube.com", "youtu.be")
}

func isTikTokURL(link string) bool {
	return matchesHost(link, "tiktok.com")
}

// matchesHost parses the URL and checks if its host matches
// any of the given domains (including subdomains like www.).
func matchesHost(link string, domains ...string) bool {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")

	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}
