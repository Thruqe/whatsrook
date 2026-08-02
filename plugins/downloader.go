// Downloader commands – Facebook, Instagram, Twitter/X, TikTok, Snapchat, Reddit, Pinterest, SoundCloud, Threads, Bluesky, VK, Tumblr, and Twitch media downloaders.
package commands

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"whatsrook/downloader"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

var (
	dlPromptsMu      sync.RWMutex
	pendingDLPrompts = make(map[string]string) // key: "chatJID:senderJID" -> service name
)

type dlService struct {
	Name string
	Cmd  string
}

var allDLServices = []dlService{
	{Name: "Facebook", Cmd: "facebook"},
	{Name: "Instagram", Cmd: "instagram"},
	{Name: "TikTok", Cmd: "tiktok"},
	{Name: "Twitter/X", Cmd: "twitter"},
	{Name: "Snapchat", Cmd: "snapchat"},
	{Name: "Reddit", Cmd: "reddit"},
	{Name: "Pinterest", Cmd: "pinterest"},
	{Name: "SoundCloud", Cmd: "soundcloud"},
	{Name: "Threads", Cmd: "threads"},
	{Name: "Bluesky", Cmd: "bluesky"},
	{Name: "VK", Cmd: "vk"},
	{Name: "Tumblr", Cmd: "tumblr"},
	{Name: "Twitch", Cmd: "twitch"},
}

func init() {
	Register(&Command{
		Name:        "downloader",
		Aliases:     []string{"dl", "download"},
		Description: "Download media from Facebook, Instagram, Twitter/X, TikTok, Snapchat, Reddit, Pinterest, SoundCloud, Threads, Bluesky, VK, Tumblr, or Twitch",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDownload,
	})
	Register(&Command{
		Name:        "facebook",
		Aliases:     []string{"fb"},
		Description: "Download video or reel from Facebook URL",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDownload,
	})
	Register(&Command{
		Name:        "instagram",
		Aliases:     []string{"ig"},
		Description: "Download post, reel, or TV from Instagram URL",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDownload,
	})
	Register(&Command{
		Name:        "twitter",
		Aliases:     []string{"x", "twt"},
		Description: "Download media from Twitter/X post URL",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDownload,
	})
	Register(&Command{
		Name:        "tiktok",
		Aliases:     []string{"tt"},
		Description: "Download video, photo slide, or audio from TikTok URL",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDownload,
	})
	Register(&Command{
		Name:        "snapchat",
		Aliases:     []string{"snap"},
		Description: "Download spotlight or story media from Snapchat URL",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDownload,
	})
	Register(&Command{
		Name:        "reddit",
		Aliases:     []string{"rd"},
		Description: "Download media or video from Reddit URL",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDownload,
	})
	Register(&Command{
		Name:        "pinterest",
		Aliases:     []string{"pin"},
		Description: "Download video or photo from Pinterest URL",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDownload,
	})
	Register(&Command{
		Name:        "soundcloud",
		Aliases:     []string{"sc"},
		Description: "Download audio track from SoundCloud URL",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDownload,
	})
	Register(&Command{
		Name:        "threads",
		Aliases:     []string{"th"},
		Description: "Download post, video, or photo from Threads URL",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDownload,
	})
	Register(&Command{
		Name:        "bluesky",
		Aliases:     []string{"bsky"},
		Description: "Download video, photo, or GIF from Bluesky URL",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDownload,
	})
	Register(&Command{
		Name:        "vk",
		Description: "Download video from VK URL",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDownload,
	})
	Register(&Command{
		Name:        "tumblr",
		Description: "Download video or audio from Tumblr URL",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDownload,
	})
	Register(&Command{
		Name:        "twitch",
		Aliases:     []string{"clip"},
		Description: "Download video clip from Twitch URL",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleDownload,
	})
}

// HandlePendingDLReply checks if the sender has an active downloader prompt and processes their URL reply.
func HandlePendingDLReply(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	if evt == nil || evt.Message == nil {
		return false
	}
	text := utils.ExtractMessageText(evt)
	if text == "" {
		return false
	}

	key := fmt.Sprintf("%s:%s", evt.Info.Chat.String(), evt.Info.Sender.String())
	dlPromptsMu.RLock()
	serviceName, exists := pendingDLPrompts[key]
	dlPromptsMu.RUnlock()

	if !exists {
		return false
	}

	var targetURL string
	for _, field := range strings.Fields(text) {
		if strings.HasPrefix(field, "http://") || strings.HasPrefix(field, "https://") {
			targetURL = field
			break
		}
	}

	if targetURL == "" {
		return false
	}

	// Remove pending prompt
	dlPromptsMu.Lock()
	delete(pendingDLPrompts, key)
	dlPromptsMu.Unlock()

	if err := downloader.ValidateURL(targetURL); err != nil {
		fakeCtx := &Context{
			Ctx:    ctx,
			Client: client,
			Chat:   evt.Info.Chat,
			Sender: evt.Info.Sender,
			Evt:    evt,
		}
		_ = fakeCtx.Reply(fmt.Sprintf("Invalid URL for %s: %v. Please provide a valid link.", serviceName, err))
		return true
	}

	fakeCtx := &Context{
		Ctx:     ctx,
		Client:  client,
		Chat:    evt.Info.Chat,
		Sender:  evt.Info.Sender,
		Evt:     evt,
		RawArgs: targetURL,
	}

	_ = executeDownload(fakeCtx, targetURL)
	return true
}

func handleDownload(ctx *Context) error {
	args := strings.Fields(ctx.RawArgs)
	if len(args) > 0 {
		sub := strings.ToLower(args[0])
		if sub == "page" && len(args) > 1 {
			pageNum, _ := strconv.Atoi(args[1])
			if pageNum < 1 {
				pageNum = 1
			}
			return sendDLMenuPage(ctx, pageNum)
		}

		if sub == "prompt" && len(args) > 1 {
			targetService := strings.Title(args[1])
			key := fmt.Sprintf("%s:%s", ctx.Chat.String(), ctx.Sender.String())
			dlPromptsMu.Lock()
			pendingDLPrompts[key] = targetService
			dlPromptsMu.Unlock()
			return ctx.Reply(fmt.Sprintf("Drop your %s URL below to download media.", targetService))
		}
	}

	targetURL := strings.TrimSpace(ctx.RawArgs)
	if targetURL == "" && ctx.Evt != nil && ctx.Evt.Message != nil {
		quotedText := utils.GetDirectMessageText(ctx.Evt.Message)
		if quotedText != "" {
			for _, field := range strings.Fields(quotedText) {
				if strings.HasPrefix(field, "http://") || strings.HasPrefix(field, "https://") {
					targetURL = field
					break
				}
			}
		}
	}

	if targetURL == "" || (!strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://")) {
		return sendDLMenuPage(ctx, 1)
	}

	// Validate URL
	if err := downloader.ValidateURL(targetURL); err != nil {
		return ctx.Reply(fmt.Sprintf("Invalid URL: %v. Please provide a valid HTTP/HTTPS social media link.", err))
	}

	return executeDownload(ctx, targetURL)
}

func sendDLMenuPage(ctx *Context, pageNum int) error {
	const itemsPerPage = 3
	totalItems := len(allDLServices)
	totalPages := (totalItems + itemsPerPage - 1) / itemsPerPage

	if pageNum < 1 {
		pageNum = 1
	}
	if pageNum > totalPages {
		pageNum = totalPages
	}

	startIndex := (pageNum - 1) * itemsPerPage
	endIndex := startIndex + itemsPerPage
	if endIndex > totalItems {
		endIndex = totalItems
	}

	pageServices := allDLServices[startIndex:endIndex]
	p := ctx.GetPrefix()

	var sb strings.Builder
	fmt.Fprintf(&sb, "╭━━━〔 MEDIA DOWNLOADER (Page %d/%d) 〕━━━\n│ Choose a service below or drop a URL directly:\n", pageNum, totalPages)
	for _, s := range pageServices {
		fmt.Fprintf(&sb, "│ • %s (%s%s)\n", s.Name, p, s.Cmd)
	}
	sb.WriteString("╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	var buttons []struct{ ID, Text string }
	for _, s := range pageServices {
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   fmt.Sprintf("%sdl prompt %s", p, strings.ToLower(s.Cmd)),
			Text: s.Name,
		})
	}

	// Navigation buttons
	if pageNum > 1 {
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   fmt.Sprintf("%sdl page %d", p, pageNum-1),
			Text: "◀️ Prev",
		})
	}
	if pageNum < totalPages {
		buttons = append(buttons, struct{ ID, Text string }{
			ID:   fmt.Sprintf("%sdl page %d", p, pageNum+1),
			Text: "Next ▶️",
		})
	}

	return sendInteractiveButtons(ctx, sb.String(), fmt.Sprintf("%s Media Downloader", ctx.GetBotName()), buttons)
}

func executeDownload(ctx *Context, targetURL string) error {
	loader := ctx.StartLoader("Fetching media...")
	defer loader.Delete()

	res, err := downloader.Download(ctx.Ctx, targetURL)
	if err != nil {
		return ctx.Reply("Download failed: " + err.Error())
	}

	if len(res.Items) == 0 {
		return ctx.Reply("No downloadable media items found.")
	}

	httpClient := &http.Client{Timeout: 60 * time.Second}

	for i, item := range res.Items {
		if i >= 5 { // Limit max items per request to avoid flooding
			break
		}

		req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodGet, item.URL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", downloader.DefaultUserAgent)

		resp, err := httpClient.Do(req)
		if err != nil {
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil || len(data) == 0 {
			continue
		}

		caption := ""
		if res.Title != "" {
			caption = res.Title
		}

		switch item.Type {
		case "photo":
			_ = ctx.ReplyWithImage(data, "image/jpeg", caption)
		case "video", "gif":
			_ = ctx.ReplyWithVideo(data, "video/mp4", caption)
		case "audio":
			_ = ctx.ReplyWithAudio(data, "audio/mp4")
		default:
			_ = ctx.ReplyWithDocument(data, "application/octet-stream", item.Filename, caption)
		}
	}

	return nil
}
