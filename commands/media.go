package commands

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func init() {
	Register(&Command{
		Name:        "sticker",
		Aliases:     []string{"s"},
		Description: "Convert an image/video to a sticker. Optional pack metadata: sticker [author] | [pack]",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleSticker,
	})
	Register(&Command{
		Name:        "circle",
		Description: "Convert an image/video to a circular sticker. Optional pack metadata: circle [author] | [pack]",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleCircle,
	})
	Register(&Command{
		Name:        "crop",
		Description: "Convert an image/video to a square cropped sticker. Optional pack metadata: crop [author] | [pack]",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleCrop,
	})
	Register(&Command{
		Name:        "steal",
		Aliases:     []string{"take"},
		Description: "Steal/take a sticker and customize its metadata. Usage: reply to a sticker and optionally specify [author] | [pack]",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleSteal,
	})
	Register(&Command{
		Name:        "mp4",
		Description: "Convert an animated sticker/video to MP4 format",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleMP4,
	})
	Register(&Command{
		Name:        "mp3",
		Description: "Convert a video/audio to MP3 format",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleMP3,
	})
	Register(&Command{
		Name:        "mp4url",
		Description: "Download video from direct URL and send as MP4",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleMP4URL,
	})
	Register(&Command{
		Name:        "black",
		Description: "Create a black video using the audio of a video/audio file",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleBlack,
	})
	Register(&Command{
		Name:        "trim",
		Description: "Trim a video. Usage: trim [start] [end] or trim [duration]",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleTrim,
	})
	Register(&Command{
		Name:        "vv",
		Description: "Unwrap a ViewOnce message and resend it as a normal message (replying to a ViewOnce message)",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleVV,
	})
}

func handleSticker(ctx *Context) error {
	data, mimetype, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("❌ No media found in this message or the replied message.")
	}

	packName, author := parseStickerMetadata(ctx.RawArgs)
	isVideo := strings.HasPrefix(mimetype, "video") || strings.Contains(mimetype, "gif")

	_ = ctx.Reply("⏳ Processing sticker...")
	stickerData, err := processSticker(ctx, data, isVideo, packName, author, "")
	if err != nil {
		return ctx.Reply(fmt.Sprintf("❌ Failed to process sticker: %v", err))
	}

	return ctx.ReplyWithSticker(stickerData)
}

func handleCircle(ctx *Context) error {
	data, mimetype, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("❌ No media found in this message or the replied message.")
	}

	packName, author := parseStickerMetadata(ctx.RawArgs)
	isVideo := strings.HasPrefix(mimetype, "video") || strings.Contains(mimetype, "gif")

	_ = ctx.Reply("⏳ Processing circular sticker...")
	// apply transparent circle mask using ffmpeg's geq/alpha filter
	circleFilter := "format=yuva420p,scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=black@0,geq=alpha_expr='if(lte(hypot(X-W/2,Y-H/2),W/2),255,0)'"
	stickerData, err := processSticker(ctx, data, isVideo, packName, author, circleFilter)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("❌ Failed to process circular sticker: %v", err))
	}

	return ctx.ReplyWithSticker(stickerData)
}

func handleCrop(ctx *Context) error {
	data, mimetype, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("❌ No media found in this message or the replied message.")
	}

	packName, author := parseStickerMetadata(ctx.RawArgs)
	isVideo := strings.HasPrefix(mimetype, "video") || strings.Contains(mimetype, "gif")

	_ = ctx.Reply("⏳ Processing cropped sticker...")
	// crop to square first, then scale
	cropFilter := "crop='min(iw,ih)':'min(iw,ih)',scale=512:512"
	stickerData, err := processSticker(ctx, data, isVideo, packName, author, cropFilter)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("❌ Failed to process cropped sticker: %v", err))
	}

	return ctx.ReplyWithSticker(stickerData)
}

func handleMP4(ctx *Context) error {
	data, _, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("❌ No media found in this message or the replied message.")
	}

	_ = ctx.Reply("⏳ Converting to MP4...")
	mp4Data, err := processMP4(data)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("❌ Failed to convert to MP4: %v", err))
	}

	return ctx.ReplyWithVideo(mp4Data, "video/mp4", "")
}

func handleMP3(ctx *Context) error {
	data, _, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("❌ No media found in this message or the replied message.")
	}

	_ = ctx.Reply("⏳ Converting to MP3...")
	mp3Data, err := processMP3(data)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("❌ Failed to convert to MP3: %v", err))
	}

	return ctx.ReplyWithAudio(mp3Data, "audio/mp3")
}

func handleMP4URL(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply("❌ Please provide a direct video URL.")
	}
	videoURL := ctx.Args[0]

	_ = ctx.Reply("⏳ Downloading and converting video...")
	videoBytes, err := downloadFromURL(ctx.Ctx, videoURL)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("❌ Failed to download video: %v", err))
	}

	mp4Data, err := processMP4(videoBytes)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("❌ Failed to process video into MP4: %v", err))
	}

	return ctx.ReplyWithVideo(mp4Data, "video/mp4", "")
}

func handleBlack(ctx *Context) error {
	data, _, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("❌ No media found in this message or the replied message.")
	}

	_ = ctx.Reply("⏳ Creating black video...")
	blackData, err := processBlackVideo(data)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("❌ Failed to create black video: %v", err))
	}

	return ctx.ReplyWithVideo(blackData, "video/mp4", "")
}

func parseStickerMetadata(raw string) (string, string) {
	packName := "WhatsRook"
	author := "Thruqe"
	if raw != "" {
		parts := strings.Split(raw, "|")
		if len(parts) > 0 {
			author = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 {
			packName = strings.TrimSpace(parts[1])
		}
	}
	return packName, author
}

func handleSteal(ctx *Context) error {
	quoted := ctx.GetQuotedMessage()
	if quoted == nil || quoted.StickerMessage == nil {
		return ctx.Reply("❌ Please reply to a sticker message.")
	}

	data, mimetype, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply(fmt.Sprintf("❌ Failed to get sticker media: %v", err))
	}

	if !strings.Contains(mimetype, "webp") {
		return ctx.Reply("❌ The replied message is not a valid sticker (WebP).")
	}

	packName, author := parseStickerMetadata(ctx.RawArgs)

	_ = ctx.Reply("⏳ Remapping sticker metadata...")

	updatedData, err := AddStickerMetadata(data, packName, author)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("❌ Failed to update sticker metadata: %v", err))
	}

	return ctx.ReplyWithSticker(updatedData)
}

func processSticker(ctx *Context, data []byte, isVideo bool, packName, author string, filter string) ([]byte, error) {
	tempIn, err := os.CreateTemp("", "sticker_in_*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempIn.Name())
	if _, err := tempIn.Write(data); err != nil {
		return nil, err
	}
	tempIn.Close()

	tempOut := tempIn.Name() + ".out.webp"
	defer os.Remove(tempOut)

	if isVideo {
		// Define encoding attempts with decreasing quality/fps/preset settings to fit under 500KB (512,000 bytes)
		type attempt struct {
			fps     int
			quality int
		}
		attempts := []attempt{
			{fps: 15, quality: 40},
			{fps: 12, quality: 30},
			{fps: 10, quality: 20},
			{fps: 7, quality: 10},
		}

		var lastErr error
		var finalData []byte

		for idx, att := range attempts {
			_ = os.Remove(tempOut)

			// Formulate the filter
			vf := fmt.Sprintf("fps=%d,format=yuva420p,scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=black@0", att.fps)
			if filter != "" {
				vf = filter
				if strings.Contains(vf, "fps=") {
					vf = strings.ReplaceAll(vf, "fps=15", fmt.Sprintf("fps=%d", att.fps))
				} else {
					vf = fmt.Sprintf("fps=%d,", att.fps) + vf
				}
				if !strings.Contains(vf, "format=yuva420p") {
					vf = "format=yuva420p," + vf
				}
			}

			cmd := exec.Command("ffmpeg", "-y", "-i", tempIn.Name(), "-t", "8", "-vf", vf, "-vcodec", "libwebp", "-lossless", "0", "-q:v", fmt.Sprintf("%d", att.quality), "-compression_level", "6", "-loop", "0", "-preset", "default", "-an", "-vsync", "0", "-pix_fmt", "yuva420p", tempOut)
			if out, err := cmd.CombinedOutput(); err != nil {
				lastErr = fmt.Errorf("ffmpeg failed at attempt %d (fps=%d, q=%d): %w (output: %s)", idx, att.fps, att.quality, err, string(out))
				continue
			}

			// Add sticker metadata
			finalPath, err := writeStickerMetadata(tempOut, packName, author)
			if err != nil {
				lastErr = fmt.Errorf("failed to write sticker metadata at attempt %d: %w", idx, err)
				continue
			}

			data, err := os.ReadFile(finalPath)
			_ = os.Remove(finalPath)
			if err != nil {
				lastErr = fmt.Errorf("failed to read final sticker path at attempt %d: %w", idx, err)
				continue
			}

			// Check size (500KB limit)
			if len(data) <= 500*1024 {
				return data, nil
			}

			// Keep track of the last encoded one in case all attempts exceed 500KB
			finalData = data
		}

		if finalData != nil {
			return finalData, nil
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("failed to process video sticker")
	} else {
		vf := "format=yuva420p,scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=black@0"
		if filter != "" {
			vf = filter
			if !strings.Contains(vf, "format=yuva420p") {
				vf = "format=yuva420p," + vf
			}
		}
		cmd := exec.Command("ffmpeg", "-y", "-i", tempIn.Name(), "-vf", vf, "-vcodec", "libwebp", "-lossless", "0", "-q:v", "40", "-compression_level", "6", "-pix_fmt", "yuva420p", tempOut)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("ffmpeg failed: %w (output: %s)", err, string(out))
		}

		finalPath, err := writeStickerMetadata(tempOut, packName, author)
		if err != nil {
			return nil, err
		}
		defer os.Remove(finalPath)

		return os.ReadFile(finalPath)
	}
}

func processMP4(data []byte) ([]byte, error) {
	tempIn, err := os.CreateTemp("", "mp4_in_*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempIn.Name())
	if _, err := tempIn.Write(data); err != nil {
		return nil, err
	}
	tempIn.Close()

	tempOut := tempIn.Name() + ".mp4"
	defer os.Remove(tempOut)

	cmd := exec.Command("ffmpeg", "-y", "-i", tempIn.Name(), "-pix_fmt", "yuv420p", "-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2", tempOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg mp4 failed: %w (output: %s)", err, string(out))
	}

	return os.ReadFile(tempOut)
}

func processMP3(data []byte) ([]byte, error) {
	tempIn, err := os.CreateTemp("", "mp3_in_*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempIn.Name())
	if _, err := tempIn.Write(data); err != nil {
		return nil, err
	}
	tempIn.Close()

	tempOut := tempIn.Name() + ".mp3"
	defer os.Remove(tempOut)

	cmd := exec.Command("ffmpeg", "-y", "-i", tempIn.Name(), "-q:a", "2", tempOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg mp3 failed: %w (output: %s)", err, string(out))
	}

	return os.ReadFile(tempOut)
}

func downloadFromURL(ctx context.Context, mediaURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, resp.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func processBlackVideo(data []byte) ([]byte, error) {
	tempIn, err := os.CreateTemp("", "black_in_*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempIn.Name())
	if _, err := tempIn.Write(data); err != nil {
		return nil, err
	}
	tempIn.Close()

	tempOut := tempIn.Name() + ".mp4"
	defer os.Remove(tempOut)

	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "color=c=black:s=640x360:d=600", "-i", tempIn.Name(), "-map", "0:v", "-map", "1:a", "-c:v", "libx264", "-tune", "stillimage", "-c:a", "aac", "-pix_fmt", "yuv420p", "-shortest", tempOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg black failed: %w (output: %s)", err, string(out))
	}

	return os.ReadFile(tempOut)
}

func handleTrim(ctx *Context) error {
	data, _, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("❌ No media found in this message or the replied message.")
	}

	if len(ctx.Args) == 0 {
		return ctx.Reply("❌ Usage: trim [start] [end] (e.g. trim 00:00:02 00:00:10) or trim [duration] (e.g. trim 10)")
	}

	start := "00:00:00"
	end := ctx.Args[0]
	if len(ctx.Args) > 1 {
		start = ctx.Args[0]
		end = ctx.Args[1]
	}

	_ = ctx.Reply(fmt.Sprintf("⏳ Trimming video from %s to %s...", start, end))
	trimmedData, err := processTrim(data, start, end)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("❌ Failed to trim video: %v", err))
	}

	return ctx.ReplyWithVideo(trimmedData, "video/mp4", "")
}

func processTrim(data []byte, start, end string) ([]byte, error) {
	tempIn, err := os.CreateTemp("", "trim_in_*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tempIn.Name())
	if _, err := tempIn.Write(data); err != nil {
		return nil, err
	}
	tempIn.Close()

	tempOut := tempIn.Name() + ".mp4"
	defer os.Remove(tempOut)

	cmd := exec.Command("ffmpeg", "-y", "-i", tempIn.Name(), "-ss", start, "-to", end, "-c:v", "libx264", "-c:a", "aac", "-pix_fmt", "yuv420p", tempOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg trim failed: %w (output: %s)", err, string(out))
	}

	return os.ReadFile(tempOut)
}

func handleVV(ctx *Context) error {
	quoted := ctx.GetQuotedMessage()
	if quoted == nil {
		return ctx.Reply("❌ Please reply to a ViewOnce message.")
	}

	isViewOnce := false
	if quoted.ViewOnceMessage != nil || quoted.ViewOnceMessageV2 != nil || quoted.ViewOnceMessageV2Extension != nil {
		isViewOnce = true
	} else if img := quoted.GetImageMessage(); img != nil && img.GetViewOnce() {
		isViewOnce = true
	} else if vid := quoted.GetVideoMessage(); vid != nil && vid.GetViewOnce() {
		isViewOnce = true
	}

	if !isViewOnce {
		return ctx.Reply("❌ The replied message is not a ViewOnce message.")
	}

	unwrapped := ExtractViewOnceMessage(quoted)
	if unwrapped == nil {
		return ctx.Reply("❌ Failed to unwrap ViewOnce message.")
	}

	// Link back to the original ViewOnce message as a quote/reply
	var quotedStanzaID string
	var quotedParticipant string
	if ext := ctx.Evt.Message.GetExtendedTextMessage(); ext != nil {
		if ci := ext.GetContextInfo(); ci != nil {
			if ci.StanzaID != nil {
				quotedStanzaID = *ci.StanzaID
			}
			if ci.Participant != nil {
				quotedParticipant = *ci.Participant
			}
		}
	}

	if quotedStanzaID != "" && quotedParticipant != "" {
		quotedClone := proto.Clone(quoted).(*waE2E.Message)

		// Clear any context info inside the cloned quoted message to break circular references completely!
		if quotedClone.ImageMessage != nil {
			quotedClone.ImageMessage.ContextInfo = nil
		}
		if quotedClone.VideoMessage != nil {
			quotedClone.VideoMessage.ContextInfo = nil
		}
		if quotedClone.AudioMessage != nil {
			quotedClone.AudioMessage.ContextInfo = nil
		}

		ci := &waE2E.ContextInfo{
			StanzaID:      &quotedStanzaID,
			Participant:   &quotedParticipant,
			QuotedMessage: quotedClone,
		}

		// Also clone unwrapped.ImageMessage / VideoMessage / AudioMessage to prevent modifying the original quoted message!
		if unwrapped.ImageMessage != nil {
			newImg := proto.Clone(unwrapped.ImageMessage).(*waE2E.ImageMessage)
			newImg.ContextInfo = ci
			unwrapped.ImageMessage = newImg
		} else if unwrapped.VideoMessage != nil {
			newVid := proto.Clone(unwrapped.VideoMessage).(*waE2E.VideoMessage)
			newVid.ContextInfo = ci
			unwrapped.VideoMessage = newVid
		} else if unwrapped.AudioMessage != nil {
			newAud := proto.Clone(unwrapped.AudioMessage).(*waE2E.AudioMessage)
			newAud.ContextInfo = ci
			unwrapped.AudioMessage = newAud
		}
	}

	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, unwrapped)
	return err
}
