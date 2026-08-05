package plugins

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"whatsrook/downloader"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
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

	dl := downloader.NewClient()
	cookiePath := filepath.Join(GetSessionAuthDir(ctx.Client), "cookies.txt")
	dl.CookieFile = cookiePath

	res, err := dl.DownloadYouTubeMedia(ctx.Ctx, targetURL, false)
	if err != nil {
		if isBotDetectionError(err.Error()) {
			return SendYTCookieHelp(ctx)
		}
		return ctx.Reply("YouTube video download failed: " + err.Error())
	}

	if len(res.Items) == 0 {
		return ctx.Reply("No downloadable video stream found.")
	}

	item := res.Items[0]
	data, err := fetchStreamBytes(ctx, item)
	if err != nil || len(data) == 0 {
		return ctx.Reply("Failed to fetch video stream.")
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

	dl := downloader.NewClient()
	cookiePath := filepath.Join(GetSessionAuthDir(ctx.Client), "cookies.txt")
	dl.CookieFile = cookiePath

	res, err := dl.DownloadYouTubeMedia(ctx.Ctx, targetURL, true)
	if err != nil {
		if isBotDetectionError(err.Error()) {
			return SendYTCookieHelp(ctx)
		}
		return ctx.Reply("YouTube audio download failed: " + err.Error())
	}

	if len(res.Items) == 0 {
		return ctx.Reply("No downloadable audio/video stream found.")
	}

	item := res.Items[0]
	data, err := fetchStreamBytes(ctx, item)
	if err != nil || len(data) == 0 {
		return ctx.Reply("Failed to fetch media stream.")
	}

	tmpIn := filepath.Join(os.TempDir(), fmt.Sprintf("yt_in_%d.bin", time.Now().UnixNano()))
	tmpOut := filepath.Join(os.TempDir(), fmt.Sprintf("yt_out_%d.m4a", time.Now().UnixNano()))

	if err := os.WriteFile(tmpIn, data, 0644); err != nil {
		return ctx.Reply("Failed to write temporary media file.")
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
// title, author (as body), and thumbnail. MediaType is left unset (nil) since
// it's optional and the exact enum constant name varies by whatsmeow version.
func replyWithMusicAudio(ctx *Context, data []byte, mimetype string, res *downloader.Result, thumb []byte) error {
	uploaded, err := ctx.Client.Upload(ctx.Ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		return fmt.Errorf("audio upload failed: %w", err)
	}

	title := res.Title
	if title == "" {
		title = "YouTube Audio"
	}

	mediaType := waE2E.ContextInfo_ExternalAdReplyInfo_IMAGE.Enum()

	adInfo := &waE2E.ContextInfo_ExternalAdReplyInfo{
		Title:                 proto.String(title),
		SourceURL:             proto.String("https://www.youtube.com/watch?v=" + res.ID),
		MediaType:             mediaType,
		RenderLargerThumbnail: proto.Bool(true),
		ShowAdAttribution:     proto.Bool(false),
	}
	if res.Author != "" {
		adInfo.Body = proto.String(res.Author)
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
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    &fileLength,
			ContextInfo:   cinfo,
		},
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
