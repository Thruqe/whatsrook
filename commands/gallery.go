package commands

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

var defaultImageUrls = []string{
	"https://i.ibb.co/0jg98zcf/RD32353537373832373130353540732e77686174736170702e6e6574-402631.jpg",
	"https://i.ibb.co/WvfKPNYy/RD32353537373832373130353540732e77686174736170702e6e6574-378431.jpg",
	"https://i.ibb.co/WNp4kTDN/RD32353537373832373130353540732e77686174736170702e6e6574-447586.jpg",
	"https://i.ibb.co/Pzfb84RP/RD32353537373832373130353540732e77686174736170702e6e6574-859675.jpg",
	"https://i.ibb.co/R4Y32jJL/RD32353537373832373130353540732e77686174736170702e6e6574-146626.jpg",
}

func init() {
	Register(&Command{
		Name:        "gallery",
		Description: "Send an interactive image gallery carousel",
		Category:    "interactive",
		IsPublic:    true,
		Handler:     handleGallery,
	})
}

type downloadResult struct {
	index int
	data  []byte
	url   string
	err   error
}

func handleGallery(ctx *Context) error {
	urls := defaultImageUrls
	if len(ctx.Args) > 0 {
		urls = ctx.Args
	}

	slog.Info("Starting gallery download & upload", "count", len(urls))

	var wg sync.WaitGroup
	results := make([]downloadResult, len(urls))

	for i, u := range urls {
		wg.Add(1)
		go func(index int, urlStr string) {
			defer wg.Done()
			data, err := downloadImage(ctx.Ctx, urlStr)
			results[index] = downloadResult{
				index: index,
				data:  data,
				url:   urlStr,
				err:   err,
			}
		}(i, u)
	}

	wg.Wait()

	var cards []*waE2E.InteractiveMessage
	var uploadErrors []error

	for _, res := range results {
		if res.err != nil {
			slog.Warn("Failed to download image", "url", res.url, "err", res.err)
			uploadErrors = append(uploadErrors, res.err)
			continue
		}

		uploaded, err := ctx.Client.Upload(ctx.Ctx, res.data, whatsmeow.MediaImage)
		if err != nil {
			slog.Warn("Failed to upload image to WA server", "url", res.url, "err", err)
			uploadErrors = append(uploadErrors, err)
			continue
		}

		card := &waE2E.InteractiveMessage{
			Header: &waE2E.InteractiveMessage_Header{
				HasMediaAttachment: new(true),
				Media: &waE2E.InteractiveMessage_Header_ImageMessage{
					ImageMessage: &waE2E.ImageMessage{
						URL:           new(uploaded.URL),
						DirectPath:    new(uploaded.DirectPath),
						MediaKey:      uploaded.MediaKey,
						Mimetype:      new("image/jpeg"),
						FileEncSHA256: uploaded.FileEncSHA256,
						FileSHA256:    uploaded.FileSHA256,
						FileLength:    new(uint64(len(res.data))),
					},
				},
				Title:    new(fmt.Sprintf("Image %d", res.index+1)),
				Subtitle: new("ABZTECH Gallery"),
			},
			Body: &waE2E.InteractiveMessage_Body{
				Text: new("Tap the button below to view the full image."),
			},
			Footer: &waE2E.InteractiveMessage_Footer{
				Text: new(fmt.Sprintf("%d/%d", res.index+1, len(urls))),
			},
			InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
				NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
					Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
						{
							Name:             new("cta_url"),
							ButtonParamsJSON: new(fmt.Sprintf(`{"display_text":"View Image","url":"%s","merchant_url":"%s"}`, res.url, res.url)),
						},
					},
				},
			},
		}
		cards = append(cards, card)
	}

	if len(cards) == 0 {
		return ctx.Reply(fmt.Sprintf(" Failed to load gallery images. Errors: %v", uploadErrors))
	}

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: &waE2E.InteractiveMessage{
					Body: &waE2E.InteractiveMessage_Body{
						Text: new("Gallery\n\nSwipe left or right to browse the images."),
					},
					Footer: &waE2E.InteractiveMessage_Footer{
						Text: new("Powered by Thruqe"),
					},
					InteractiveMessage: &waE2E.InteractiveMessage_CarouselMessage_{
						CarouselMessage: &waE2E.InteractiveMessage_CarouselMessage{
							Cards: cards,
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

	slog.Info("Relaying gallery message", "cards_count", len(cards))
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	return err
}

func downloadImage(ctx context.Context, urlStr string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
