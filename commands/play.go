// Play command – search for a song using go-ytdlp and send interactive buttons to choose video or audio format.
package commands

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lrstanley/go-ytdlp"
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func init() {
	Register(&Command{
		Name:        "play",
		Description: "Search YouTube for a song and download as video or audio via interactive buttons",
		Category:    "media",
		IsPublic:    true,
		Handler:     handlePlay,
	})
}

func handlePlay(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply("Please specify a song name.")
	}

	firstArg := strings.ToLower(ctx.Args[0])
	if (firstArg == "video" || firstArg == "audio") && len(ctx.Args) >= 2 {
		return handlePlayDownload(ctx, firstArg, strings.Join(ctx.Args[1:], " "))
	}

	songQuery := ctx.RawArgs
	if songQuery == "" {
		songQuery = strings.Join(ctx.Args, " ")
	}

	cmd := ytdlp.New().
		PrintJSON().
		SkipDownload().
		FlatPlaylist()

	if cookiePath, cleanupCookie, ok := GetYouTubeCookieFile(ctx); ok {
		defer cleanupCookie()
		cmd.Cookies(cookiePath)
	}

	res, err := cmd.Run(ctx.Ctx, "ytsearch1:"+songQuery)
	if err != nil {
		slog.Error("play search failed", "query", songQuery, "err", err)
		return ctx.Reply("Search failed for the requested song.")
	}

	infos, err := res.GetExtractedInfo()
	if err != nil || len(infos) == 0 {
		slog.Warn("play search returned no info", "query", songQuery, "err", err)
		return ctx.Reply("No results found for that song.")
	}

	info := infos[0]
	if len(info.Entries) > 0 && info.Entries[0] != nil {
		info = info.Entries[0]
	}

	videoID := info.ID
	if videoID == "" && info.WebpageURL != nil {
		videoID = *info.WebpageURL
	}
	if videoID == "" {
		return ctx.Reply("Could not resolve video details for that song.")
	}

	title := "Unknown Title"
	if info.Title != nil && *info.Title != "" {
		title = *info.Title
	}

	durationStr := "Unknown"
	if info.Duration != nil && *info.Duration > 0 {
		d := time.Duration(*info.Duration) * time.Second
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		durationStr = fmt.Sprintf("%02d:%02d", m, s)
	}

	uploaderStr := ""
	if info.Uploader != nil && *info.Uploader != "" {
		uploaderStr = *info.Uploader
	} else if info.Channel != nil && *info.Channel != "" {
		uploaderStr = *info.Channel
	}

	bodyText := fmt.Sprintf("Title: %s\nDuration: %s", title, durationStr)
	if uploaderStr != "" {
		bodyText = fmt.Sprintf("Title: %s\nChannel: %s\nDuration: %s", title, uploaderStr, durationStr)
	}
	bodyText += "\n\nWould you like to download this as video or audio?"

	prefix := ctx.GetPrefix()
	videoBtnID := fmt.Sprintf("%splay video %s", prefix, videoID)
	audioBtnID := fmt.Sprintf("%splay audio %s", prefix, videoID)

	videoBtnJSON := fmt.Sprintf(`{"display_text":"Video","id":%q}`, videoBtnID)
	audioBtnJSON := fmt.Sprintf(`{"display_text":"Audio","id":%q}`, audioBtnID)

	msgVersion := int32(1)

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: &waE2E.InteractiveMessage{
					Body: &waE2E.InteractiveMessage_Body{
						Text: &bodyText,
					},
					Footer: &waE2E.InteractiveMessage_Footer{
						Text: new("Powered by WhatsRook"),
					},
					InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
						NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
							Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
								{
									Name:             new("quick_reply"),
									ButtonParamsJSON: &videoBtnJSON,
								},
								{
									Name:             new("quick_reply"),
									ButtonParamsJSON: &audioBtnJSON,
								},
							},
							MessageVersion: &msgVersion,
						},
					},
				},
			},
		},
	}

	bizNode := waBinary.Node{
		Tag:   "biz",
		Attrs: waBinary.Attrs{},
		Content: []waBinary.Node{
			{
				Tag: "interactive",
				Attrs: waBinary.Attrs{
					"type": "native_flow",
					"v":    "1",
				},
				Content: []waBinary.Node{
					{
						Tag: "native_flow",
						Attrs: waBinary.Attrs{
							"v":    "9",
							"name": "mixed",
						},
					},
				},
			},
		},
	}

	extra := whatsmeow.SendRequestExtra{
		AdditionalNodes: &[]waBinary.Node{bizNode},
	}

	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	return err
}

func handlePlayDownload(ctx *Context, format string, target string) error {
	targetURL := target
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		targetURL = "https://www.youtube.com/watch?v=" + target
	}

	_ = ctx.Reply("Downloading " + format + "...")

	tmpDir := os.TempDir()

	if format == "audio" {
		rawPath := filepath.Join(tmpDir, fmt.Sprintf("play_aud_raw_%d", time.Now().UnixNano()))

		cmdAud := ytdlp.New().
			ExtractAudio().
			Output(rawPath + ".%(ext)s")

		if cookiePath, cleanupCookie, ok := GetYouTubeCookieFile(ctx); ok {
			defer cleanupCookie()
			cmdAud.Cookies(cookiePath)
		}

		_, err := cmdAud.Run(ctx.Ctx, targetURL)
		if err != nil {
			slog.Error("play audio download failed", "target", targetURL, "err", err)
			return ctx.Reply("Failed to download audio.")
		}

		matches, _ := filepath.Glob(rawPath + ".*")
		if len(matches) == 0 {
			slog.Error("play raw audio file not found", "pattern", rawPath+".*")
			return ctx.Reply("Failed to locate downloaded audio file.")
		}
		downloadedFile := matches[0]
		defer os.Remove(downloadedFile)

		outMP3 := filepath.Join(tmpDir, fmt.Sprintf("play_%d.mp3", time.Now().UnixNano()))
		defer os.Remove(outMP3)

		ffmpegCmd := exec.CommandContext(ctx.Ctx, "ffmpeg", "-y", "-i", downloadedFile,
			"-vn", "-c:a", "libmp3lame", "-b:a", "192k", "-ar", "44100", outMP3)

		if out, err := ffmpegCmd.CombinedOutput(); err != nil {
			slog.Warn("ffmpeg mp3 transcode failed, trying fallback direct read", "err", err, "out", string(out))
			audData, rErr := os.ReadFile(downloadedFile)
			if rErr != nil || len(audData) == 0 {
				return ctx.Reply("Failed to process audio file.")
			}
			return ctx.ReplyWithAudio(audData, "audio/mp3")
		}

		audData, err := os.ReadFile(outMP3)
		if err != nil || len(audData) == 0 {
			slog.Error("play audio read failed", "path", outMP3, "err", err)
			return ctx.Reply("Failed to read processed audio file.")
		}

		return ctx.ReplyWithAudio(audData, "audio/mp3")
	}

	// Video download
	rawPath := filepath.Join(tmpDir, fmt.Sprintf("play_vid_raw_%d", time.Now().UnixNano()))

	cmdVid := ytdlp.New().
		Format("bestvideo+bestaudio/best").
		Output(rawPath + ".%(ext)s")

	if cookiePath, cleanupCookie, ok := GetYouTubeCookieFile(ctx); ok {
		defer cleanupCookie()
		cmdVid.Cookies(cookiePath)
	}

	_, err := cmdVid.Run(ctx.Ctx, targetURL)
	if err != nil {
		slog.Warn("play video download standard format failed, retrying with default format", "target", targetURL, "err", err)
		cmdFallback := ytdlp.New().Output(rawPath + ".%(ext)s")
		if cookiePath, cleanupCookie, ok := GetYouTubeCookieFile(ctx); ok {
			defer cleanupCookie()
			cmdFallback.Cookies(cookiePath)
		}
		_, err = cmdFallback.Run(ctx.Ctx, targetURL)
		if err != nil {
			slog.Error("play video download failed", "target", targetURL, "err", err)
			return ctx.Reply("Failed to download video.")
		}
	}

	matches, _ := filepath.Glob(rawPath + ".*")
	if len(matches) == 0 {
		slog.Error("play raw video file not found", "pattern", rawPath+".*")
		return ctx.Reply("Failed to locate downloaded video file.")
	}
	downloadedFile := matches[0]
	defer os.Remove(downloadedFile)

	outMP4 := filepath.Join(tmpDir, fmt.Sprintf("play_%d.mp4", time.Now().UnixNano()))
	defer os.Remove(outMP4)

	ffmpegCmd := exec.CommandContext(ctx.Ctx, "ffmpeg", "-y", "-i", downloadedFile,
		"-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "-profile:v", "main", "-level:v", "4.0",
		"-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart",
		outMP4)

	if out, err := ffmpegCmd.CombinedOutput(); err != nil {
		slog.Warn("ffmpeg video transcode failed, falling back to direct file read", "err", err, "out", string(out))
		vidData, rErr := os.ReadFile(downloadedFile)
		if rErr == nil && len(vidData) > 0 {
			return ctx.ReplyWithVideo(vidData, "video/mp4", "")
		}
		return ctx.Reply("Failed to transcode video to WhatsApp compatible format.")
	}

	vidData, err := os.ReadFile(outMP4)
	if err != nil || len(vidData) == 0 {
		slog.Error("play video read failed", "path", outMP4, "err", err)
		return ctx.Reply("Failed to read processed video file.")
	}

	return ctx.ReplyWithVideo(vidData, "video/mp4", "")
}
