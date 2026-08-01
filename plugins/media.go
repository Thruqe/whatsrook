// Media download command – downloads images/video/audio/gif from URLs.
package commands

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"whatsrook/utils"
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
}

func handleSticker(ctx *Context) error {
	data, mimetype, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("No media found in this message or the replied message.")
	}

	packName, author := parseStickerMetadata(ctx, ctx.RawArgs)
	isVideo := strings.HasPrefix(mimetype, "video") || strings.Contains(mimetype, "gif")

	loader := ctx.StartLoader("Processing sticker")
	defer loader.Delete()

	stickerData, err := processSticker(data, isVideo, packName, author, "")
	if err != nil {
		loader.Delete()
		return ctx.Reply(fmt.Sprintf(" Failed to process sticker: %v", err))
	}

	return ctx.ReplyWithSticker(stickerData)
}

func handleCircle(ctx *Context) error {
	data, mimetype, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("No media found in this message or the replied message.")
	}

	packName, author := parseStickerMetadata(ctx, ctx.RawArgs)
	isVideo := strings.HasPrefix(mimetype, "video") || strings.Contains(mimetype, "gif")

	loader := ctx.StartLoader("Processing circular sticker")
	defer loader.Delete()

	// apply transparent circle mask using ffmpeg's geq/alpha filter
	circleFilter := "format=yuva420p,scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=black@0,geq=alpha_expr='if(lte(hypot(X-W/2,Y-H/2),W/2),255,0)'"
	stickerData, err := processSticker(data, isVideo, packName, author, circleFilter)
	if err != nil {
		loader.Delete()
		return ctx.Reply(fmt.Sprintf(" Failed to process circular sticker: %v", err))
	}

	return ctx.ReplyWithSticker(stickerData)
}

func handleCrop(ctx *Context) error {
	data, mimetype, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("No media found in this message or the replied message.")
	}

	packName, author := parseStickerMetadata(ctx, ctx.RawArgs)
	isVideo := strings.HasPrefix(mimetype, "video") || strings.Contains(mimetype, "gif")

	loader := ctx.StartLoader("Processing cropped sticker")
	defer loader.Delete()

	// crop to square first, then scale
	cropFilter := "crop='min(iw,ih)':'min(iw,ih)',scale=512:512"
	stickerData, err := processSticker(data, isVideo, packName, author, cropFilter)
	if err != nil {
		loader.Delete()
		return ctx.Reply(fmt.Sprintf(" Failed to process cropped sticker: %v", err))
	}

	return ctx.ReplyWithSticker(stickerData)
}

func handleMP4(ctx *Context) error {
	data, _, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("No media found in this message or the replied message.")
	}

	loader := ctx.StartLoader("Converting to MP4")
	defer loader.Delete()

	mp4Data, err := processMP4(data)
	if err != nil {
		loader.Delete()
		return ctx.Reply(fmt.Sprintf(" Failed to convert to MP4: %v", err))
	}

	return ctx.ReplyWithVideo(mp4Data, "video/mp4", "")
}

func handleMP3(ctx *Context) error {
	data, _, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("No media found in this message or the replied message.")
	}

	loader := ctx.StartLoader("Converting to MP3")
	defer loader.Delete()

	mp3Data, err := processMP3(data)
	if err != nil {
		loader.Delete()
		return ctx.Reply(fmt.Sprintf(" Failed to convert to MP3: %v", err))
	}

	return ctx.ReplyWithAudio(mp3Data, "audio/mp3")
}

func handleMP4URL(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply("Please provide a direct video URL.")
	}
	videoURL := ctx.Args[0]

	loader := ctx.StartLoader("Downloading video")
	defer loader.Delete()

	videoBytes, err := downloadFromURL(ctx.Ctx, videoURL)
	if err != nil {
		loader.Delete()
		return ctx.Reply(fmt.Sprintf(" Failed to download video: %v", err))
	}

	mp4Data, err := processMP4(videoBytes)
	if err != nil {
		loader.Delete()
		return ctx.Reply(fmt.Sprintf(" Failed to process video into MP4: %v", err))
	}

	return ctx.ReplyWithVideo(mp4Data, "video/mp4", "")
}

func handleBlack(ctx *Context) error {
	data, _, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("No media found in this message or the replied message.")
	}

	loader := ctx.StartLoader("Creating black video")
	defer loader.Delete()

	blackData, err := processBlackVideo(data)
	if err != nil {
		loader.Delete()
		return ctx.Reply(fmt.Sprintf(" Failed to create black video: %v", err))
	}

	return ctx.ReplyWithVideo(blackData, "video/mp4", "")
}

func parseStickerMetadata(ctx *Context, raw string) (string, string) {
	packName := ctx.GetBotName()
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
	return author, packName
}

func handleSteal(ctx *Context) error {
	quoted := ctx.GetQuotedMessage()
	if quoted == nil || quoted.StickerMessage == nil {
		return ctx.Reply("Please reply to a sticker message.")
	}

	data, mimetype, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to get sticker media: %v", err))
	}

	if !strings.Contains(mimetype, "webp") {
		return ctx.Reply("The replied message is not a valid sticker (WebP).")
	}

	packName, author := parseStickerMetadata(ctx, ctx.RawArgs)

	loader := ctx.StartLoader("Remapping sticker metadata")
	defer loader.Delete()

	updatedData, err := utils.AddStickerMetadata(data, packName, author)
	if err != nil {
		loader.Delete()
		return ctx.Reply(fmt.Sprintf(" Failed to update sticker metadata: %v", err))
	}

	return ctx.ReplyWithSticker(updatedData)
}

func processSticker(data []byte, isVideo bool, packName, author, filter string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "whatsrook_sticker_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	tempIn := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(tempIn, data, 0644); err != nil {
		return nil, err
	}

	tempOut := filepath.Join(tmpDir, "output.webp")

	if isVideo {
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

			cmd := exec.Command("ffmpeg", "-y", "-i", tempIn, "-t", "8", "-vf", vf, "-vcodec", "libwebp", "-lossless", "0", "-q:v", fmt.Sprintf("%d", att.quality), "-compression_level", "6", "-loop", "0", "-preset", "default", "-an", "-vsync", "0", "-pix_fmt", "yuva420p", tempOut)
			if out, err := cmd.CombinedOutput(); err != nil {
				lastErr = fmt.Errorf("ffmpeg failed at attempt %d (fps=%d, q=%d): %w (output: %s)", idx, att.fps, att.quality, err, string(out))
				continue
			}

			finalPath, err := utils.WriteStickerMetadata(tempOut, packName, author)
			if err != nil {
				lastErr = fmt.Errorf("sticker metadata failed at attempt %d: %w", idx, err)
				continue
			}

			data, err := os.ReadFile(finalPath)
			_ = os.Remove(finalPath)
			if err != nil {
				lastErr = fmt.Errorf("read failed at attempt %d: %w", idx, err)
				continue
			}

			if len(data) <= 500*1024 {
				return data, nil
			}
			finalData = data
		}

		if finalData != nil {
			return finalData, nil
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("failed to process video sticker")
	}

	vf := "format=yuva420p,scale=512:512:force_original_aspect_ratio=decrease,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=black@0"
	if filter != "" {
		vf = filter
		if !strings.Contains(vf, "format=yuva420p") {
			vf = "format=yuva420p," + vf
		}
	}
	cmd := exec.Command("ffmpeg", "-y", "-i", tempIn, "-vf", vf, "-vcodec", "libwebp", "-lossless", "0", "-q:v", "40", "-compression_level", "6", "-pix_fmt", "yuva420p", tempOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w (output: %s)", err, string(out))
	}

	finalPath, err := utils.WriteStickerMetadata(tempOut, packName, author)
	if err != nil {
		return nil, err
	}
	defer os.Remove(finalPath)

	return os.ReadFile(finalPath)
}

func processMP4(data []byte) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "whatsrook_mp4_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	tempIn := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(tempIn, data, 0644); err != nil {
		return nil, err
	}

	tempOut := filepath.Join(tmpDir, "output.mp4")

	cmd := exec.Command("ffmpeg", "-y", "-i", tempIn, "-pix_fmt", "yuv420p", "-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2", tempOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg mp4 failed: %w (output: %s)", err, string(out))
	}

	return os.ReadFile(tempOut)
}

func processMP3(data []byte) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "whatsrook_mp3_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	tempIn := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(tempIn, data, 0644); err != nil {
		return nil, err
	}

	tempOut := filepath.Join(tmpDir, "output.mp3")

	cmd := exec.Command("ffmpeg", "-y", "-i", tempIn, "-q:a", "2", tempOut)
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

	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, resp.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func processBlackVideo(data []byte) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "whatsrook_black_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	tempIn := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(tempIn, data, 0644); err != nil {
		return nil, err
	}

	tempOut := filepath.Join(tmpDir, "output.mp4")

	cmd := exec.Command("ffmpeg", "-y", "-f", "lavfi", "-i", "color=c=black:s=640x360:d=600", "-i", tempIn, "-map", "0:v", "-map", "1:a", "-c:v", "libx264", "-tune", "stillimage", "-c:a", "aac", "-pix_fmt", "yuv420p", "-shortest", tempOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg black failed: %w (output: %s)", err, string(out))
	}

	return os.ReadFile(tempOut)
}

func handleTrim(ctx *Context) error {
	data, _, err := ctx.GetMedia()
	if err != nil {
		return ctx.Reply("No media found in this message or the replied message.")
	}

	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage: trim [start] [end] (e.g. trim 00:00:02 00:00:10) or trim [duration] (e.g. trim 10)")
	}

	start := "00:00:00"
	end := ctx.Args[0]
	if len(ctx.Args) > 1 {
		start = ctx.Args[0]
		end = ctx.Args[1]
	}

	_ = ctx.Reply(fmt.Sprintf(" Trimming video from %s to %s...", start, end))
	trimmedData, err := processTrim(data, start, end)
	if err != nil {
		return ctx.Reply(fmt.Sprintf(" Failed to trim video: %v", err))
	}

	return ctx.ReplyWithVideo(trimmedData, "video/mp4", "")
}

func processTrim(data []byte, start, end string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "whatsrook_trim_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	tempIn := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(tempIn, data, 0644); err != nil {
		return nil, err
	}

	tempOut := filepath.Join(tmpDir, "output.mp4")

	cmd := exec.Command("ffmpeg", "-y", "-i", tempIn, "-ss", start, "-to", end, "-c:v", "libx264", "-c:a", "aac", "-pix_fmt", "yuv420p", tempOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg trim failed: %w (output: %s)", err, string(out))
	}

	return os.ReadFile(tempOut)
}
