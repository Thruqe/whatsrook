// YouTube commands – .ytv (YouTube Video) and .yta (YouTube Audio with ffmpeg encoding).
package commands

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"whatsrook/downloader"
	"whatsrook/utils"
)

func init() {
	Register(&Command{
		Name:        "ytv",
		Aliases:     []string{"ytvideo", "youtubevideo"},
		Description: "Download video from YouTube URL",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleYTV,
	})

	Register(&Command{
		Name:        "yta",
		Aliases:     []string{"ytaudio", "youtubeaudio"},
		Description: "Download audio track from YouTube URL and convert with ffmpeg for WhatsApp playback",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleYTA,
	})
}

func handleYTV(ctx *Context) error {
	targetURL := extractTargetURL(ctx)
	if targetURL == "" {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: %sytv <YouTube URL>", p))
	}

	if err := downloader.ValidateURL(targetURL); err != nil {
		return ctx.Reply(fmt.Sprintf("Invalid YouTube URL: %v", err))
	}

	loader := ctx.StartLoader("Fetching YouTube video...")
	defer loader.Delete()

	res, err := downloader.Download(ctx.Ctx, targetURL)
	if err != nil {
		return ctx.Reply("YouTube video download failed: " + err.Error())
	}

	if len(res.Items) == 0 {
		return ctx.Reply("No downloadable video stream found.")
	}

	httpClient := &http.Client{Timeout: 60 * time.Second}
	item := res.Items[0]

	req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return ctx.Reply("Failed to request video stream: " + err.Error())
	}
	req.Header.Set("User-Agent", downloader.DefaultUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return ctx.Reply("Failed to fetch video stream: " + err.Error())
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) == 0 {
		return ctx.Reply("Downloaded video data was empty.")
	}

	caption := res.Title
	if caption == "" {
		caption = "YouTube Video"
	}

	return ctx.ReplyWithVideo(data, "video/mp4", caption)
}

func handleYTA(ctx *Context) error {
	targetURL := extractTargetURL(ctx)
	if targetURL == "" {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: %syta <YouTube URL>", p))
	}

	if err := downloader.ValidateURL(targetURL); err != nil {
		return ctx.Reply(fmt.Sprintf("Invalid YouTube URL: %v", err))
	}

	loader := ctx.StartLoader("Fetching and encoding YouTube audio...")
	defer loader.Delete()

	res, err := downloader.Download(ctx.Ctx, targetURL)
	if err != nil {
		return ctx.Reply("YouTube audio download failed: " + err.Error())
	}

	if len(res.Items) == 0 {
		return ctx.Reply("No downloadable audio/video stream found.")
	}

	httpClient := &http.Client{Timeout: 60 * time.Second}
	item := res.Items[0]

	req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return ctx.Reply("Failed to request media stream: " + err.Error())
	}
	req.Header.Set("User-Agent", downloader.DefaultUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return ctx.Reply("Failed to fetch media stream: " + err.Error())
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) == 0 {
		return ctx.Reply("Downloaded media data was empty.")
	}

	// Save raw data to temp file for ffmpeg conversion
	tmpIn := fmt.Sprintf("/tmp/yt_in_%d.bin", time.Now().UnixNano())
	tmpOut := fmt.Sprintf("/tmp/yt_out_%d.m4a", time.Now().UnixNano())

	if err := os.WriteFile(tmpIn, data, 0644); err != nil {
		return ctx.Reply("Failed to write temporary media file.")
	}
	defer os.Remove(tmpIn)
	defer os.Remove(tmpOut)

	// Use ffmpeg to convert to a WhatsApp compatible AAC audio track (.m4a / audio/mp4)
	cmd := exec.CommandContext(ctx.Ctx, "ffmpeg", "-y", "-i", tmpIn, "-vn", "-c:a", "aac", "-b:a", "128k", tmpOut)
	if err := cmd.Run(); err != nil {
		// Fallback: send raw downloaded data as audio if ffmpeg fails
		return ctx.ReplyWithAudio(data, "audio/mp4")
	}

	audioBytes, err := os.ReadFile(tmpOut)
	if err != nil || len(audioBytes) == 0 {
		return ctx.ReplyWithAudio(data, "audio/mp4")
	}

	return ctx.ReplyWithAudio(audioBytes, "audio/mp4")
}

func extractTargetURL(ctx *Context) string {
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
	return targetURL
}
