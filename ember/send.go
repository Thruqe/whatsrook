package ember

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

var downloadClient = &http.Client{Timeout: 60 * time.Second}

// downloadBytes pulls the raw media file from the CDN URL Ember gave us.
func downloadBytes(ctx context.Context, mediaURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := downloadClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("media download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("media download returned status %d", resp.StatusCode)
	}

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, resp.Body); err != nil {
		return nil, fmt.Errorf("media read failed: %w", err)
	}
	return buf.Bytes(), nil
}

// SendResult downloads the media from an Ember Data result, uploads it to
// WhatsApp, and sends it as a video/image message with caption to the chat.
func SendResult(ctx context.Context, client *whatsmeow.Client, chat types.JID, data *Data) error {
	media, ok := data.BestMedia()
	if !ok {
		return fmt.Errorf("no media found in ember response for %s", data.Source)
	}

	bytesData, err := downloadBytes(ctx, media.URL)
	if err != nil {
		return err
	}

	caption := data.Caption()

	switch media.Type {
	case "video":
		uploaded, err := client.Upload(ctx, bytesData, whatsmeow.MediaVideo)
		if err != nil {
			return fmt.Errorf("upload failed: %w", err)
		}
		msg := &waE2E.Message{
			VideoMessage: &waE2E.VideoMessage{
				URL:           new(uploaded.URL),
				DirectPath:    new(uploaded.DirectPath),
				MediaKey:      uploaded.MediaKey,
				Mimetype:      new("video/mp4"),
				FileEncSHA256: uploaded.FileEncSHA256,
				FileSHA256:    uploaded.FileSHA256,
				FileLength:    new(uint64(len(bytesData))),
				Caption:       new(caption),
			},
		}
		_, err = client.SendMessage(ctx, chat, msg)
		return err

	case "image":
		uploaded, err := client.Upload(ctx, bytesData, whatsmeow.MediaImage)
		if err != nil {
			return fmt.Errorf("upload failed: %w", err)
		}
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
		return err

	default:
		return fmt.Errorf("unsupported media type: %s", media.Type)
	}
}
