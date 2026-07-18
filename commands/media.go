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
)

func init() {
	Register(&Command{
		Name:        "sticker",
		Aliases:     []string{"s"},
		Description: "Convert an image/video to a sticker. Optional pack metadata: sticker [pack] | [author]",
		Category:    "media",
		IsPublic:     true,
		Handler:     handleSticker,
	})
	Register(&Command{
		Name:        "circle",
		Description: "Convert an image/video to a circular sticker. Optional pack metadata: circle [pack] | [author]",
		Category:    "media",
		IsPublic:     true,
		Handler:     handleCircle,
	})
	Register(&Command{
		Name:        "crop",
		Description: "Convert an image/video to a square cropped sticker. Optional pack metadata: crop [pack] | [author]",
		Category:    "media",
		IsPublic:     true,
		Handler:     handleCrop,
	})
	Register(&Command{
		Name:        "mp4",
		Description: "Convert an animated sticker/video to MP4 format",
		Category:    "media",
		IsPublic:     true,
		Handler:     handleMP4,
	})
	Register(&Command{
		Name:        "mp3",
		Description: "Convert a video/audio to MP3 format",
		Category:    "media",
		IsPublic:     true,
		Handler:     handleMP3,
	})
	Register(&Command{
		Name:        "mp4url",
		Description: "Download video from direct URL and send as MP4",
		Category:    "media",
		IsPublic:     true,
		Handler:     handleMP4URL,
	})
	Register(&Command{
		Name:        "black",
		Description: "Create a black video using the audio of a video/audio file",
		Category:    "media",
		IsPublic:     true,
		Handler:     handleBlack,
	})
	Register(&Command{
		Name:        "trim",
		Description: "Trim a video. Usage: trim [start] [end] or trim [duration]",
		Category:    "media",
		IsPublic:     true,
		Handler:     handleTrim,
	})
	Register(&Command{
		Name:        "vv",
		Description: "Unwrap a ViewOnce message and resend it as a normal message (replying to a ViewOnce message)",
		Category:    "media",
		IsPublic:     true,
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
	packName := "WhatsRook Pack"
	author := "WhatsRook Bot"
	if raw != "" {
		parts := strings.Split(raw, "|")
		if len(parts) > 0 {
			packName = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 {
			author = strings.TrimSpace(parts[1])
		}
	}
	return packName, author
}

func generateStickerExif(packName, author string) []byte {
	jsonStr := fmt.Sprintf(`{"sticker-pack-id":"whatsrook.pack","sticker-pack-name":%q,"sticker-pack-publisher":%q}`, packName, author)
	jsonBytes := []byte(jsonStr)
	jsonLen := uint32(len(jsonBytes))

	exifHeader := []byte("Exif\x00\x00")
	tiffHeader := []byte("II\x2a\x00\x08\x00\x00\x00")
	numEntries := []byte{0x01, 0x00}

	entry := make([]byte, 12)
	entry[0] = 0x41
	entry[1] = 0x57
	entry[2] = 0x07
	entry[3] = 0x00
	entry[4] = byte(jsonLen)
	entry[5] = byte(jsonLen >> 8)
	entry[6] = byte(jsonLen >> 16)
	entry[7] = byte(jsonLen >> 24)
	entry[8] = 0x1A
	entry[9] = 0x00
	entry[10] = 0x00
	entry[11] = 0x00

	nextIFD := []byte{0x00, 0x00, 0x00, 0x00}

	var buf bytes.Buffer
	buf.Write(exifHeader)
	buf.Write(tiffHeader)
	buf.Write(numEntries)
	buf.Write(entry)
	buf.Write(nextIFD)
	buf.Write(jsonBytes)

	return buf.Bytes()
}

func writeStickerMetadata(inputPath, packName, author string) (string, error) {
	exifBytes := generateStickerExif(packName, author)
	exifFile, err := os.CreateTemp("", "sticker_exif_*.exif")
	if err != nil {
		return "", fmt.Errorf("failed to create exif temp file: %w", err)
	}
	defer os.Remove(exifFile.Name())

	if _, err := exifFile.Write(exifBytes); err != nil {
		exifFile.Close()
		return "", fmt.Errorf("failed to write exif bytes: %w", err)
	}
	exifFile.Close()

	outputPath := inputPath + ".metadata.webp"
	cmd := exec.Command("webpmux", "-set", "exif", exifFile.Name(), inputPath, "-o", outputPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("webpmux failed: %w (output: %s)", err, string(out))
	}

	return outputPath, nil
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

	var cmd *exec.Cmd
	if isVideo {
		vf := "scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=black@0"
		if filter != "" {
			vf = filter
		}
		cmd = exec.Command("ffmpeg", "-y", "-i", tempIn.Name(), "-t", "6", "-vf", vf, "-vcodec", "libwebp", "-lossless", "0", "-q:v", "50", "-loop", "0", "-preset", "default", "-an", "-vsync", "0", tempOut)
	} else {
		vf := "scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=black@0"
		if filter != "" {
			vf = filter
		}
		cmd = exec.Command("ffmpeg", "-y", "-i", tempIn.Name(), "-vf", vf, "-vcodec", "libwebp", "-lossless", "0", "-q:v", "50", tempOut)
	}

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
		// Clone quoted message by doing a shallow copy of its main struct
		quotedClone := *quoted

		// Clear any context info inside the cloned quoted message to break circular references completely!
		if quotedClone.ImageMessage != nil {
			clonedImg := *quotedClone.ImageMessage
			clonedImg.ContextInfo = nil
			quotedClone.ImageMessage = &clonedImg
		}
		if quotedClone.VideoMessage != nil {
			clonedVid := *quotedClone.VideoMessage
			clonedVid.ContextInfo = nil
			quotedClone.VideoMessage = &clonedVid
		}
		if quotedClone.AudioMessage != nil {
			clonedAud := *quotedClone.AudioMessage
			clonedAud.ContextInfo = nil
			quotedClone.AudioMessage = &clonedAud
		}

		ci := &waE2E.ContextInfo{
			StanzaID:      &quotedStanzaID,
			Participant:   &quotedParticipant,
			QuotedMessage: &quotedClone,
		}

		// Also clone unwrapped.ImageMessage / VideoMessage / AudioMessage to prevent modifying the original quoted message!
		if unwrapped.ImageMessage != nil {
			newImg := *unwrapped.ImageMessage
			newImg.ContextInfo = ci
			unwrapped.ImageMessage = &newImg
		} else if unwrapped.VideoMessage != nil {
			newVid := *unwrapped.VideoMessage
			newVid.ContextInfo = ci
			unwrapped.VideoMessage = &newVid
		} else if unwrapped.AudioMessage != nil {
			newAud := *unwrapped.AudioMessage
			newAud.ContextInfo = ci
			unwrapped.AudioMessage = &newAud
		}
	}

	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, unwrapped)
	return err
}

