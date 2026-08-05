// Play command – searches YouTube for music, presents selection or downloads audio with external ad context & thumbnail.
package plugins

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"whatsrook/downloader"
)

func init() {
	Register(&Command{
		Name:        "play",
		Aliases:     []string{"song", "music", "ytplay"},
		Description: "Search YouTube for music and play audio with title & thumbnail",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handlePlay,
	})

	Register(&Command{
		Name:        "yt",
		Aliases:     []string{"youtube"},
		Description: "Download video or audio from YouTube URL or query",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handlePlay,
	})
}

func handlePlay(ctx *Context) error {
	p := ctx.GetPrefix()
	query := strings.TrimSpace(ctx.RawArgs)
	if query == "" {
		return ctx.Reply(fmt.Sprintf("Usage: %splay <song title or search query>\nExample: %splay Faded Alan Walker", p, p))
	}

	// Check if input is direct YouTube URL
	if strings.HasPrefix(query, "http://") || strings.HasPrefix(query, "https://") {
		return processYouTubeAudioDownload(ctx, query)
	}

	loader := ctx.StartLoader(fmt.Sprintf("Searching YouTube for %q...", query))
	defer loader.Delete()

	dl := downloader.NewClient()
	cookiePath := filepath.Join(GetSessionAuthDir(ctx.Client), "cookies.txt")
	dl.CookieFile = cookiePath

	results, err := dl.Search(ctx.Ctx, query, 5, "ytsearch")
	if err != nil {
		slog.Error("handlePlay: YouTube search failed", "err", err)
		if isBotDetectionError(err.Error()) {
			return SendYTCookieHelp(ctx)
		}
		return ctx.Reply(fmt.Sprintf("YouTube search failed: %v", err))
	}

	if len(results) == 0 {
		return ctx.Reply(fmt.Sprintf("No YouTube songs found matching %q.", query))
	}

	// If single result, process audio directly
	if len(results) == 1 {
		return processYouTubeAudioDownload(ctx, results[0].GetURL())
	}

	// Send interactive selection buttons for top 3 search results
	var buttons []struct{ ID, Text string }
	var sb strings.Builder
	fmt.Fprintf(&sb, "*YOUTUBE SEARCH RESULTS*\n\nQuery: _%s_\n\n", query)

	for i, item := range results {
		if i >= 3 {
			break
		}
		num := i + 1
		itemURL := item.GetURL()
		if itemURL == "" {
			continue
		}
		title := item.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		fmt.Fprintf(&sb, "%d. *%s*\n", num, item.Title)
		btnID := fmt.Sprintf("%syta %s", p, itemURL)
		btnText := fmt.Sprintf("Download #%d", num)
		buttons = append(buttons, struct{ ID, Text string }{ID: btnID, Text: btnText})
	}

	sb.WriteString("\nClick a button below to download your audio:")
	return sendInteractiveButtons(ctx, sb.String(), fmt.Sprintf("%s Music Downloader", ctx.GetBotName()), buttons)
}

func processYouTubeAudioDownload(ctx *Context, targetURL string) error {
	loader := ctx.StartLoader("Downloading YouTube audio...")
	defer loader.Delete()

	dl := downloader.NewClient()
	cookiePath := filepath.Join(GetSessionAuthDir(ctx.Client), "cookies.txt")
	dl.CookieFile = cookiePath

	res, err := dl.DownloadYouTubeMedia(ctx.Ctx, targetURL, true)
	if err != nil {
		slog.Error("processYouTubeAudioDownload: YouTube download failed", "url", targetURL, "err", err)
		if isBotDetectionError(err.Error()) {
			return SendYTCookieHelp(ctx)
		}
		return ctx.Reply(fmt.Sprintf("YouTube download failed: %v", err))
	}

	if len(res.Items) == 0 {
		return ctx.Reply("No downloadable audio stream found.")
	}

	item := res.Items[0]
	data, err := fetchStreamBytes(ctx, item)
	if err != nil || len(data) == 0 {
		return ctx.Reply("Failed to fetch audio stream bytes.")
	}

	tmpIn := filepath.Join(os.TempDir(), fmt.Sprintf("yt_in_%d.bin", time.Now().UnixNano()))
	tmpOut := filepath.Join(os.TempDir(), fmt.Sprintf("yt_out_%d.m4a", time.Now().UnixNano()))

	if err := os.WriteFile(tmpIn, data, 0644); err != nil {
		return ctx.Reply("Failed to write temporary audio file.")
	}
	defer os.Remove(tmpIn)
	defer os.Remove(tmpOut)

	cmd := exec.CommandContext(ctx.Ctx, "ffmpeg", "-y", "-i", tmpIn, "-vn", "-c:a", "aac", "-b:a", "128k", tmpOut)
	audioBytes := data
	mimetype := "audio/mp4"
	if err := cmd.Run(); err == nil {
		if converted, rerr := os.ReadFile(tmpOut); rerr == nil && len(converted) > 0 {
			audioBytes = converted
		}
	}

	// Fetch thumbnail with fallback
	thumb := fetchThumbnailBytes(ctx, res.Thumbnail)
	if len(thumb) == 0 && res.ID != "" {
		fallbackURL := fmt.Sprintf("https://img.youtube.com/vi/%s/hqdefault.jpg", res.ID)
		thumb = fetchThumbnailBytes(ctx, fallbackURL)
	}

	return replyWithMusicAudio(ctx, audioBytes, mimetype, res, thumb)
}

func isBotDetectionError(errStr string) bool {
	lower := strings.ToLower(errStr)
	return strings.Contains(lower, "sign in to confirm") ||
		strings.Contains(lower, "bot") ||
		strings.Contains(lower, "captcha") ||
		strings.Contains(lower, "cookie")
}
