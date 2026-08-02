// Downloader commands – Facebook, Instagram, Twitter/X, TikTok media downloaders.
package commands

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"whatsrook/downloader"
	"whatsrook/utils"
)

func init() {
	Register(&Command{
		Name:        "downloader",
		Aliases:     []string{"dl", "download"},
		Description: "Download media from Facebook, Instagram, Twitter/X, or TikTok",
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

func handleDownload(ctx *Context) error {
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

	if targetURL == "" {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: %sdl <URL>\n\nSupported services:\n- Facebook (%sfb)\n- Instagram (%sig)\n- Twitter/X (%sx / %stwt)\n- TikTok (%stt)", p, p, p, p, p, p))
	}

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
