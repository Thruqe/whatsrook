// Low-level message sending functions: text, media, stickers, location, reactions.
package sender

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"whatsrook/ember"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

var downloadClient = &http.Client{Timeout: 60 * time.Second}

// downloadBytes pulls the raw media file from the CDN URL Ember gave us.
func downloadBytes(ctx context.Context, mediaURL string) ([]byte, error) {
	slog.Debug("ember.downloadBytes: starting download", "url", mediaURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		slog.Error("ember.downloadBytes: failed to create request", "err", err)
		return nil, err
	}
	resp, err := downloadClient.Do(req)
	if err != nil {
		slog.Error("ember.downloadBytes: HTTP request failed", "err", err)
		return nil, fmt.Errorf("media download failed: %w", err)
	}
	defer resp.Body.Close()

	slog.Debug("ember.downloadBytes: HTTP response received", "status_code", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		slog.Warn("ember.downloadBytes: non-OK status code received", "status_code", resp.StatusCode)
		return nil, fmt.Errorf("media download returned status %d", resp.StatusCode)
	}

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, resp.Body); err != nil {
		slog.Error("ember.downloadBytes: failed to copy response body", "err", err)
		return nil, fmt.Errorf("media read failed: %w", err)
	}
	slog.Debug("ember.downloadBytes: download completed", "bytes_read", buf.Len())
	return buf.Bytes(), nil
}

// SendResult downloads the media from an Ember Data result, uploads it to
// WhatsApp, and sends it as a video/image message with caption to the chat.
func SendResult(ctx context.Context, client *whatsmeow.Client, chat types.JID, data *ember.Data) error {
	slog.Debug("ember.SendResult: starting", "chat", chat.String())
	media, ok := data.BestMedia()
	if !ok {
		slog.Warn("ember.SendResult: no best media found")
		return fmt.Errorf("no media found in ember response for %s", data.Source)
	}

	slog.Debug("ember.SendResult: best media found", "url", media.URL, "type", media.Type)
	bytesData, err := downloadBytes(ctx, media.URL)
	if err != nil {
		slog.Error("ember.SendResult: download failed", "err", err)
		return err
	}

	caption := data.Caption()
	slog.Debug("ember.SendResult: uploading media to WhatsApp", "type", media.Type, "caption", caption)

	switch media.Type {
	case "video":
		convBytes, err := transcodeVideo(ctx, bytesData)
		if err != nil {
			slog.Warn("ember.SendResult: transcodeVideo error (ignored, falling back to original)", "err", err)
			convBytes = bytesData
		}

		uploaded, err := client.Upload(ctx, convBytes, whatsmeow.MediaVideo)
		if err != nil {
			slog.Error("ember.SendResult: WhatsApp upload failed", "err", err)
			return fmt.Errorf("upload failed: %w", err)
		}
		slog.Debug("ember.SendResult: WhatsApp upload success, sending message")
		msg := &waE2E.Message{
			VideoMessage: &waE2E.VideoMessage{
				URL:           new(uploaded.URL),
				DirectPath:    new(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      new("video/mp4"),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    new(uint64(len(convBytes))),
				Caption:       new(caption),
			},
		}
		_, err = client.SendMessage(ctx, chat, msg)
		if err != nil {
			slog.Error("ember.SendResult: SendMessage failed", "err", err)
		} else {
			slog.Debug("ember.SendResult: SendMessage success")
		}
		return err

	case "image":
		uploaded, err := client.Upload(ctx, bytesData, whatsmeow.MediaImage)
		if err != nil {
			slog.Error("ember.SendResult: WhatsApp upload failed", "err", err)
			return fmt.Errorf("upload failed: %w", err)
		}
		slog.Debug("ember.SendResult: WhatsApp upload success, sending message")
		msg := &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				URL:           new(uploaded.URL),
				DirectPath:    new(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      new("image/jpeg"),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    new(uint64(len(bytesData))),
				Caption:       new(caption),
			},
		}
		_, err = client.SendMessage(ctx, chat, msg)
		if err != nil {
			slog.Error("ember.SendResult: SendMessage failed", "err", err)
		} else {
			slog.Debug("ember.SendResult: SendMessage success")
		}
		return err

	case "audio":
		convBytes, err := transcodeAudio(ctx, bytesData)
		if err != nil {
			slog.Warn("ember.SendResult: transcodeAudio error (ignored, falling back to original)", "err", err)
			convBytes = bytesData
		}

		uploaded, err := client.Upload(ctx, convBytes, whatsmeow.MediaAudio)
		if err != nil {
			slog.Error("ember.SendResult: WhatsApp upload failed", "err", err)
			return fmt.Errorf("upload failed: %w", err)
		}
		slog.Debug("ember.SendResult: WhatsApp upload success, sending audio message")
		msg := &waE2E.Message{
			AudioMessage: &waE2E.AudioMessage{
				URL:           new(uploaded.URL),
				DirectPath:    new(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      new("audio/mp4"),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    new(uint64(len(convBytes))),
			},
		}
		_, err = client.SendMessage(ctx, chat, msg)
		if err != nil {
			slog.Error("ember.SendResult: SendMessage failed", "err", err)
		} else {
			slog.Debug("ember.SendResult: SendMessage success")
		}
		return err

	default:
		slog.Error("ember.SendResult: unsupported media type", "type", media.Type)
		return fmt.Errorf("unsupported media type: %s", media.Type)
	}
}

func transcodeVideo(ctx context.Context, inputData []byte) ([]byte, error) {
	slog.Debug("ember.transcodeVideo: starting transcoding via ffmpeg")

	tmpDir, err := os.MkdirTemp("", "whatsrook_ember_vid_*")
	if err != nil {
		slog.Error("ember.transcodeVideo: failed to create temp dir", "err", err)
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpIn := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(tmpIn, inputData, 0644); err != nil {
		slog.Error("ember.transcodeVideo: failed to write input", "err", err)
		return nil, fmt.Errorf("failed to write input: %w", err)
	}

	tmpOutName := filepath.Join(tmpDir, "output.mp4")

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", tmpIn,
		"-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "-profile:v", "main", "-level:v", "4.0",
		"-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart",
		tmpOutName)

	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Error("ffmpeg video transcode failed", "err", err, "output", string(out))
		return nil, fmt.Errorf("ffmpeg video transcode failed: %w (output: %s)", err, string(out))
	}

	convertedData, err := os.ReadFile(tmpOutName)
	if err != nil {
		slog.Error("ember.transcodeVideo: failed to read converted file", "err", err)
		return nil, err
	}

	slog.Debug("ember.transcodeVideo: completed", "orig_size", len(inputData), "new_size", len(convertedData))
	return convertedData, nil
}

func transcodeAudio(ctx context.Context, inputData []byte) ([]byte, error) {
	slog.Debug("ember.transcodeAudio: starting transcoding via ffmpeg")

	tmpDir, err := os.MkdirTemp("", "whatsrook_ember_aud_*")
	if err != nil {
		slog.Error("ember.transcodeAudio: failed to create temp dir", "err", err)
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpIn := filepath.Join(tmpDir, "input")
	if err := os.WriteFile(tmpIn, inputData, 0644); err != nil {
		slog.Error("ember.transcodeAudio: failed to write input", "err", err)
		return nil, fmt.Errorf("failed to write input: %w", err)
	}

	tmpOutName := filepath.Join(tmpDir, "output.mp4")

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", tmpIn,
		"-vn", "-c:a", "aac", "-b:a", "128k",
		tmpOutName)

	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Error("ffmpeg audio transcode failed", "err", err, "output", string(out))
		return nil, fmt.Errorf("ffmpeg audio transcode failed: %w (output: %s)", err, string(out))
	}

	convertedData, err := os.ReadFile(tmpOutName)
	if err != nil {
		slog.Error("ember.transcodeAudio: failed to read converted file", "err", err)
		return nil, err
	}

	slog.Debug("ember.transcodeAudio: completed", "orig_size", len(inputData), "new_size", len(convertedData))
	return convertedData, nil
}
