package plugins

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"whatsrook/downloader"
	"whatsrook/utils"

	"whatsrook/wa-core"
	"whatsrook/wa-core/proto/waE2E"
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

	Register(&Command{
		Name:        "yts",
		Aliases:     []string{"ytsearch", "searchyt"},
		Description: "Search YouTube for videos and display formatted results",
		Category:    "downloader",
		IsPublic:    true,
		Handler:     handleYTS,
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

	dl := downloader.NewClient()
	cookiePath := filepath.Join(GetSessionAuthDir(ctx.Client), "cookies.txt")
	dl.CookieFile = cookiePath

	slog.Debug("handleYTV: Initiating YouTube video download", "url", targetURL, "cookiePath", cookiePath)

	res, err := dl.DownloadYouTubeMedia(ctx.Ctx, targetURL, false)
	if err != nil {
		slog.Warn("handleYTV: DownloadYouTubeMedia failed", "url", targetURL, "err", err)
		if isBotDetectionError(err.Error()) {
			return SendYTCookieHelp(ctx)
		}
		return ctx.Reply("YouTube video download failed: " + err.Error())
	}

	if len(res.Items) == 0 {
		slog.Warn("handleYTV: No items returned in result", "url", targetURL)
		return ctx.Reply("No downloadable video stream found.")
	}

	item := res.Items[0]
	slog.Debug("handleYTV: Fetching video stream bytes", "itemURL", item.URL, "type", item.Type)
	data, err := fetchStreamBytes(ctx, item)
	if err != nil || len(data) == 0 {
		slog.Error("handleYTV: Failed to fetch stream bytes", "err", err, "size", len(data))
		return ctx.Reply("Failed to fetch video stream.")
	}
	slog.Debug("handleYTV: Successfully fetched video stream bytes", "size", len(data))

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

	dl := downloader.NewClient()
	cookiePath := filepath.Join(GetSessionAuthDir(ctx.Client), "cookies.txt")
	dl.CookieFile = cookiePath

	slog.Debug("handleYTA: Initiating YouTube audio download", "url", targetURL, "cookiePath", cookiePath)

	res, err := dl.DownloadYouTubeMedia(ctx.Ctx, targetURL, true)
	if err != nil {
		slog.Warn("handleYTA: DownloadYouTubeMedia failed", "url", targetURL, "err", err)
		if isBotDetectionError(err.Error()) {
			return SendYTCookieHelp(ctx)
		}
		return ctx.Reply("YouTube audio download failed: " + err.Error())
	}

	if len(res.Items) == 0 {
		slog.Warn("handleYTA: No items returned in result", "url", targetURL)
		return ctx.Reply("No downloadable audio/video stream found.")
	}

	item := res.Items[0]
	slog.Debug("handleYTA: Fetching audio stream bytes", "itemURL", item.URL, "type", item.Type)
	data, err := fetchStreamBytes(ctx, item)
	if err != nil || len(data) == 0 {
		slog.Error("handleYTA: Failed to fetch media stream bytes", "err", err, "size", len(data))
		return ctx.Reply("Failed to fetch media stream.")
	}
	slog.Debug("handleYTA: Successfully fetched raw audio bytes", "size", len(data))

	tmpIn := filepath.Join(os.TempDir(), fmt.Sprintf("yt_in_%d.bin", time.Now().UnixNano()))
	tmpOut := filepath.Join(os.TempDir(), fmt.Sprintf("yt_out_%d.opus", time.Now().UnixNano()))

	if err := os.WriteFile(tmpIn, data, 0644); err != nil {
		slog.Error("handleYTA: Failed to write temp input file", "path", tmpIn, "err", err)
		return ctx.Reply("Failed to write temporary media file.")
	}
	defer os.Remove(tmpIn)
	defer os.Remove(tmpOut)

	slog.Debug("handleYTA: Executing ffmpeg libopus encoding", "tmpIn", tmpIn, "tmpOut", tmpOut)
	cmd := exec.CommandContext(ctx.Ctx, "ffmpeg", "-y", "-i", tmpIn, "-vn", "-c:a", "libopus", "-b:a", "32k", "-application", "voip", "-f", "ogg", tmpOut)
	audioBytes := data
	mimetype := "audio/ogg; codecs=opus"
	if err := cmd.Run(); err == nil {
		if converted, rerr := os.ReadFile(tmpOut); rerr == nil && len(converted) > 0 {
			audioBytes = converted
			slog.Debug("handleYTA: ffmpeg conversion succeeded", "outputSize", len(audioBytes))
		}
	} else {
		slog.Warn("handleYTA: ffmpeg conversion failed, using raw audio data", "err", err)
	}

	thumb := fetchThumbnailBytes(ctx, res.Thumbnail)
	if len(thumb) == 0 && res.ID != "" {
		fallbackURL := fmt.Sprintf("https://img.youtube.com/vi/%s/hqdefault.jpg", res.ID)
		thumb = fetchThumbnailBytes(ctx, fallbackURL)
	}

	return replyWithMusicAudio(ctx, audioBytes, mimetype, res, thumb)
}

func fetchStreamBytes(ctx *Context, item downloader.MediaItem) ([]byte, error) {
	if len(item.Buffer) > 0 {
		return item.Buffer, nil
	}
	if item.URL == "" {
		return nil, fmt.Errorf("empty item URL")
	}

	if strings.HasPrefix(item.URL, "http://") || strings.HasPrefix(item.URL, "https://") {
		httpClient := &http.Client{Timeout: 120 * time.Second}
		req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodGet, item.URL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", downloader.DefaultUserAgent)

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("stream fetch status %d", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}

	data, err := os.ReadFile(item.URL)
	if err == nil {
		defer os.Remove(item.URL)
	}
	return data, err
}

func fetchThumbnailBytes(ctx *Context, thumbURL string) []byte {
	if thumbURL == "" {
		return nil
	}
	httpClient := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodGet, thumbURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", downloader.DefaultUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, _ := io.ReadAll(resp.Body)
	return data
}

// replyWithMusicAudio sends audio with an ExternalAdReplyInfo "music card":
// title, author (as body), and thumbnail.
func replyWithMusicAudio(ctx *Context, data []byte, mimetype string, res *downloader.Result, thumb []byte) error {
	meta, errMeta := utils.EnsureOpusPTT(ctx.Ctx, data)
	if errMeta == nil && meta != nil && len(meta.Data) > 0 {
		data = meta.Data
	}
	mimetype = "audio/ogg; codecs=opus"

	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		return fmt.Errorf("audio upload failed: %w", err)
	}

	title := res.Title
	if title == "" {
		title = "YouTube Audio"
	}

	mediaType := waE2E.ContextInfo_ExternalAdReplyInfo_IMAGE.Enum()

	thumbURL := res.Thumbnail
	if thumbURL == "" && res.ID != "" {
		thumbURL = fmt.Sprintf("https://img.youtube.com/vi/%s/hqdefault.jpg", res.ID)
	}

	sourceURL := "https://github.com/Thruqe/whatsrook"
	if res.ID != "" {
		sourceURL = "https://www.youtube.com/watch?v=" + res.ID
	}

	adInfo := &waE2E.ContextInfo_ExternalAdReplyInfo{
		Title:                 new(title),
		SourceURL:             new(sourceURL),
		MediaType:             mediaType,
		RenderLargerThumbnail: new(true),
		ShowAdAttribution:     new(false),
	}
	if thumbURL != "" {
		adInfo.ThumbnailURL = new(thumbURL)
	}
	if res.Author != "" {
		adInfo.Body = new(res.Author)
	}
	if len(thumb) > 0 {
		jpegData, errConv := utils.EnsureJPEG(ctx.Ctx, thumb)
		if errConv == nil && len(jpegData) > 0 {
			adInfo.Thumbnail = jpegData
		} else {
			adInfo.Thumbnail = thumb
		}
	}

	cinfo := &waE2E.ContextInfo{
		ExternalAdReply: adInfo,
	}

	fileLength := uint64(len(data))
	msg := &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           &uploaded.URL,
			DirectPath:    &uploaded.DirectPath,
			MediaKey:      uploaded.MediaKey,
			Mimetype:      new(mimetype),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    &fileLength,
			PTT:           new(true),
			ContextInfo:   cinfo,
		},
	}
	if meta != nil {
		if meta.Seconds > 0 {
			msg.AudioMessage.Seconds = new(meta.Seconds)
		}
		if len(meta.Waveform) > 0 {
			msg.AudioMessage.Waveform = meta.Waveform
		}
	}

	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg)
	return err
}

func extractTargetURL(ctx *Context) string {
	targetURL := strings.TrimSpace(ctx.RawArgs)
	if targetURL == "" && ctx.Evt != nil && ctx.Evt.Message != nil {
		quotedText := utils.GetDirectMessageText(ctx.Evt.Message)
		if quotedText != "" {
			for field := range strings.FieldsSeq(quotedText) {
				if strings.HasPrefix(field, "http://") || strings.HasPrefix(field, "https://") {
					targetURL = field
					break
				}
			}
		}
	}
	return targetURL
}

func handleYTS(ctx *Context) error {
	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: %syts <search query>\nExample: %syts Alan Walker Faded", p, p))
	}

	query := strings.Join(ctx.Args, " ")

	loader := ctx.StartLoader(fmt.Sprintf("Searching YouTube for %q...", query))
	defer loader.Delete()

	dl := downloader.NewClient()
	cookiePath := filepath.Join(GetSessionAuthDir(ctx.Client), "cookies.txt")
	dl.CookieFile = cookiePath

	results, err := dl.Search(ctx.Ctx, query, 5, "ytsearch")
	if err != nil {
		slog.Error("handleYTS: YouTube search failed", "err", err)
		if isBotDetectionError(err.Error()) {
			return SendYTCookieHelp(ctx)
		}
		return ctx.Reply(fmt.Sprintf("YouTube search failed: %v", err))
	}

	if len(results) == 0 {
		return ctx.Reply(fmt.Sprintf("No YouTube videos found matching %q.", query))
	}

	var buttons []struct{ ID, Text string }
	var sb strings.Builder
	p := ctx.GetPrefix()

	fmt.Fprintf(&sb, "*YOUTUBE SEARCH RESULTS*\n\nQuery: _%s_\n\n", query)

	for i, item := range results {
		if i >= 3 {
			break
		}
		num := i + 1
		title := item.Title
		url := item.GetURL()
		if url == "" {
			continue
		}
		duration := item.FormatDuration()
		channel := item.Uploader
		if channel == "" {
			channel = "YouTube"
		}

		fmt.Fprintf(&sb, "%d. *%s*\n", num, title)
		if duration != "N/A" {
			fmt.Fprintf(&sb, "Duration: %s | Channel: %s\n", duration, channel)
		} else {
			fmt.Fprintf(&sb, "Channel: %s\n", channel)
		}
		fmt.Fprintf(&sb, "Link: %s\n\n", url)

		btnID := fmt.Sprintf("%sytv %s", p, url)
		btnText := fmt.Sprintf("Download Video #%d", num)
		buttons = append(buttons, struct{ ID, Text string }{ID: btnID, Text: btnText})
	}

	sb.WriteString("Select a button below to download video:")

	return sendInteractiveButtons(ctx, sb.String(), fmt.Sprintf("%s YouTube Search", ctx.GetBotName()), buttons)
}
