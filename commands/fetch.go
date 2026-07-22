package commands

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Thruqe/whatsrook/ember"
	"github.com/Thruqe/whatsrook/sender"
)

var urlPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

func init() {
	Register(&Command{
		Name:        "dl",
		Aliases:     []string{"download"},
		Description: "Download media from Instagram, TikTok, YouTube, Facebook, Threads, or Twitter/X",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDl,
	})
	Register(&Command{
		Name:        "fetch",
		Description: "Make custom HTTP requests (GET/POST/etc.) and inspect response headers/body",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleFetch,
	})
}

func handleDl(ctx *Context) error {
	slog.Info("handleDl started", "raw_args", ctx.RawArgs)
	link := resolveFetchURL(ctx)
	if link == "" {
		slog.Warn("handleDl: no URL resolved")
		return sendText(ctx, "Usage: !dl <url> (or reply to a message containing a url)")
	}
	slog.Info("handleDl: resolved URL", "url", link)
	if !isSupportedFetchURL(link) {
		slog.Warn("handleDl: URL is unsupported", "url", link)
		return sendText(ctx, "Unsupported url. Supported: Instagram, TikTok, YouTube, Facebook, Threads, Twitter/X")
	}

	var cookie string
	if isYouTubeURL(link) {
		cookie = getYouTubeCookie(ctx)
		slog.Info("handleDl: YouTube cookie retrieved", "cookie_len", len(cookie))
	}
	slog.Info("handleDl: calling ember.Fetch", "url", link)
	data, err := ember.Fetch(ctx.Ctx, link, cookie)
	if err != nil {
		slog.Error("handleDl: ember.Fetch failed", "err", err)
		return sendText(ctx, fmt.Sprintf("Failed: %s", err))
	}
	slog.Info("handleDl: ember.Fetch success, calling SendResult", "title", data.Title, "medias_count", len(data.Medias))
	return sender.SendResult(ctx.Ctx, ctx.Client, ctx.Chat, data)
}

func handleFetch(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage: fetch [METHOD] <url> [body...] [Header: Value...]")
	}

	method := "GET"
	urlStr := ""
	bodyStr := ""
	headers := make(map[string]string)

	argIdx := 0
	firstArg := strings.ToUpper(ctx.Args[argIdx])

	methods := map[string]bool{
		"GET":     true,
		"POST":    true,
		"PUT":     true,
		"DELETE":  true,
		"PATCH":   true,
		"HEAD":    true,
		"OPTIONS": true,
	}

	if methods[firstArg] {
		method = firstArg
		argIdx++
	}

	if argIdx >= len(ctx.Args) {
		return ctx.Reply("URL is required.")
	}

	urlStr = ctx.Args[argIdx]
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "https://" + urlStr
	}
	argIdx++

	// Parse remaining arguments for body and headers
	var bodyParts []string
	for i := argIdx; i < len(ctx.Args); i++ {
		arg := ctx.Args[i]
		if strings.Contains(arg, ":") {
			parts := strings.SplitN(arg, ":", 2)
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if isValidHeaderKey(key) && val != "" {
				headers[key] = val
				continue
			}
		}
		bodyParts = append(bodyParts, arg)
	}

	bodyStr = strings.Join(bodyParts, " ")

	var reqBody io.Reader
	if bodyStr != "" {
		reqBody = strings.NewReader(bodyStr)
	}

	req, err := http.NewRequestWithContext(ctx.Ctx, method, urlStr, reqBody)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to create request: %v", err))
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "whatsrook/1.0")
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" HTTP request failed: %v", err))
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to read response: %v", err))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "HTTP/1.1 %s\n", resp.Status)
	for k, vv := range resp.Header {
		for _, v := range vv {
			fmt.Fprintf(&sb, "%s: %s\n", k, v)
		}
	}
	sb.WriteString("\n")

	bodyOut := string(respBytes)
	if len(bodyOut) > 1500 {
		bodyOut = bodyOut[:1500] + "\n... (truncated)"
	}
	sb.WriteString(bodyOut)

	return ctx.Reply(sb.String())
}

func isValidHeaderKey(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}

// resolveFetchURL picks a media URL from command args, otherwise from a quoted message.
func resolveFetchURL(ctx *Context) string {
	if ctx.RawArgs != "" {
		if link := firstURL(ctx.RawArgs); link != "" {
			return link
		}
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
