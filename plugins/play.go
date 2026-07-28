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
		JsRuntimes("bun").
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
	videoBtnID := prefix + "play video " + videoID
	audioBtnID := prefix + "play audio " + videoID

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: &bodyText,
					FooterText:  new("Powered by WhatsRook"),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons: []*waE2E.ButtonsMessage_Button{
						{
							ButtonID:   new(videoBtnID),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("Video")},
							Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID:   new(audioBtnID),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("Audio")},
							Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
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

// isCookieError checks if the yt-dlp error is caused by missing/invalid cookies
// (YouTube bot detection requiring sign-in).
func isCookieError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "sign in to confirm") ||
		strings.Contains(errStr, "use --cookies-from-browser") ||
		strings.Contains(errStr, "use --cookies for the authentication") ||
		strings.Contains(errStr, "login_required")
}

// sendCookieHelp sends a helpful message with buttons guiding the user to set cookies.
func sendCookieHelp(ctx *Context) error {
	prefix := ctx.GetPrefix()
	bodyText := fmt.Sprintf("You haven't configured your YouTube cookies yet. YouTube is blocking this request because it looks like a bot.\n\nPlease check out the %scookie command for instructions, or use the %sai command for more help.", prefix, prefix)

	cookieBtnID := prefix + "cookie"
	aiBtnID := prefix + "ai"

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: &bodyText,
					FooterText:  new("Powered by WhatsRook"),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons: []*waE2E.ButtonsMessage_Button{
						{
							ButtonID:   new(cookieBtnID),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("Cookie Tutorial")},
							Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID:   new(aiBtnID),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{DisplayText: new("Ask AI")},
							Type:       waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
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

	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	return err
}

func handlePlayDownload(ctx *Context, format string, target string) error {
	// Rate limit: 30s cooldown per user
	if !dlLimiter.Acquire(ctx.Sender.String(), 30*time.Second) {
		return ctx.Reply("Please wait a moment before requesting another download.")
	}
	defer dlLimiter.Release(ctx.Sender.String())

	targetURL := target
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		targetURL = "https://www.youtube.com/watch?v=" + target
	}

	_ = ctx.Reply("Downloading " + format + "...")

	tmpDir, err := os.MkdirTemp("", "whatsrook_play_*")
	if err != nil {
		slog.Error("play: failed to create temp dir", "err", err)
		return ctx.Reply("Failed to create download directory.")
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if format == "audio" {
		rawPath := filepath.Join(tmpDir, "audio_raw")

		cmdAud := ytdlp.New().
			ExtractAudio().
			JsRuntimes("bun").
			Output(rawPath + ".%(ext)s")

		if cookiePath, cleanupCookie, ok := GetYouTubeCookieFile(ctx); ok {
			defer cleanupCookie()
			cmdAud.Cookies(cookiePath)
		}

		_, err := cmdAud.Run(ctx.Ctx, targetURL)
		if err != nil {
			slog.Error("play audio download failed", "target", targetURL, "err", err)
			if isCookieError(err) {
				return sendCookieHelp(ctx)
			}
			return ctx.Reply("Failed to download audio.")
		}

		matches, _ := filepath.Glob(rawPath + ".*")
		if len(matches) == 0 {
			slog.Error("play raw audio file not found", "pattern", rawPath+".*")
			return ctx.Reply("Failed to locate downloaded audio file.")
		}
		downloadedFile := matches[0]

		outMP4 := filepath.Join(tmpDir, "output.mp4")

		ffmpegCmd := exec.CommandContext(ctx.Ctx, "ffmpeg", "-y", "-i", downloadedFile,
			"-vn", "-c:a", "aac", "-b:a", "128k", "-ar", "44100", outMP4)

		if out, err := ffmpegCmd.CombinedOutput(); err != nil {
			slog.Warn("ffmpeg aac transcode failed, trying fallback", "err", err, "out", string(out))
			audData, rErr := os.ReadFile(downloadedFile)
			if rErr != nil || len(audData) == 0 {
				return ctx.Reply("Failed to process audio file.")
			}
			return ctx.ReplyWithAudio(audData, "audio/mp4")
		}

		audData, err := os.ReadFile(outMP4)
		if err != nil || len(audData) == 0 {
			slog.Error("play audio read failed", "path", outMP4, "err", err)
			return ctx.Reply("Failed to read processed audio file.")
		}
		return ctx.ReplyWithAudio(audData, "audio/mp4")
	}

	// Video download
	rawPath := filepath.Join(tmpDir, "video_raw")

	cmdVid := ytdlp.New().
		Format("bestvideo+bestaudio/best").
		JsRuntimes("bun").
		Output(rawPath + ".%(ext)s")

	if cookiePath, cleanupCookie, ok := GetYouTubeCookieFile(ctx); ok {
		defer cleanupCookie()
		cmdVid.Cookies(cookiePath)
	}

	_, err = cmdVid.Run(ctx.Ctx, targetURL)
	if err != nil {
		slog.Warn("play video download standard format failed, retrying", "target", targetURL, "err", err)
		if isCookieError(err) {
			return sendCookieHelp(ctx)
		}

		cmdFallback := ytdlp.New().JsRuntimes("bun").Output(rawPath + ".%(ext)s")
		if cookiePath, cleanupCookie, ok := GetYouTubeCookieFile(ctx); ok {
			defer cleanupCookie()
			cmdFallback.Cookies(cookiePath)
		}
		_, err = cmdFallback.Run(ctx.Ctx, targetURL)
		if err != nil {
			slog.Error("play video download failed", "target", targetURL, "err", err)
			if isCookieError(err) {
				return sendCookieHelp(ctx)
			}
			return ctx.Reply("Failed to download video.")
		}
	}

	matches, _ := filepath.Glob(rawPath + ".*")
	if len(matches) == 0 {
		slog.Error("play raw video file not found", "pattern", rawPath+".*")
		return ctx.Reply("Failed to locate downloaded video file.")
	}
	downloadedFile := matches[0]

	outMP4 := filepath.Join(tmpDir, "output.mp4")

	ffmpegCmd := exec.CommandContext(ctx.Ctx, "ffmpeg", "-y", "-i", downloadedFile,
		"-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "-profile:v", "main", "-level:v", "4.0",
		"-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart",
		outMP4)

	if out, err := ffmpegCmd.CombinedOutput(); err != nil {
		slog.Warn("ffmpeg video transcode failed, falling back", "err", err, "out", string(out))
		vidData, rErr := os.ReadFile(downloadedFile)
		if rErr == nil && len(vidData) > 0 {
			return ctx.ReplyWithVideo(vidData, "video/mp4", "")
		}
		return ctx.Reply("Failed to transcode video.")
	}

	vidData, err := os.ReadFile(outMP4)
	if err != nil || len(vidData) == 0 {
		slog.Error("play video read failed", "path", outMP4, "err", err)
		return ctx.Reply("Failed to read processed video file.")
	}
	return ctx.ReplyWithVideo(vidData, "video/mp4", "")
}
