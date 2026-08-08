// Screenshot (ss) plugin – captures a high-resolution website screenshot and sends it as a photo.
package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func init() {
	Register(&Command{
		Name:        "ss",
		Aliases:     []string{"screenshot", "webss", "webshot", "shot"},
		Description: "Capture a high-resolution screenshot of a website URL",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleScreenshot,
	})
}

var ssHttpClient = &http.Client{
	Timeout: 25 * time.Second,
}

func handleScreenshot(ctx *Context) error {
	query := strings.TrimSpace(ctx.RawArgs)
	if query == "" {
		if quoted := ctx.GetQuotedMessage(); quoted != nil {
			query = strings.TrimSpace(extractTextFromProto(quoted))
		}
	}

	if query == "" {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage: `%sss <URL>`\n\nExample:\n- `%sss https://google.com`\n- Reply to a message containing a URL with `%sss`", p, p, p))
	}

	// Extract first URL word if text contains multiple words
	fields := strings.Fields(query)
	targetURL := ""
	for _, f := range fields {
		if strings.Contains(f, ".") && !strings.HasPrefix(f, "@") {
			targetURL = f
			break
		}
	}
	if targetURL == "" {
		targetURL = fields[0]
	}

	targetURL = strings.TrimPrefix(targetURL, "<")
	targetURL = strings.TrimSuffix(targetURL, ">")

	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Host == "" || !strings.Contains(parsed.Host, ".") {
		return ctx.Reply("Invalid URL. Please specify a valid web address (e.g. `https://google.com`).")
	}

	loader := ctx.StartLoader("Capturing website screenshot...")
	defer loader.Delete()

	imgData, mimeType, err := fetchWebsiteScreenshot(ctx.Ctx, targetURL)
	if err != nil {
		slog.Error("handleScreenshot failed", "url", targetURL, "err", err)
		return ctx.Reply(fmt.Sprintf("Failed to capture screenshot for `%s`.\nPlease verify the website URL and try again.", targetURL))
	}

	caption := fmt.Sprintf("📸 *Website Screenshot*\n\n🌐 *URL:* %s\n⚡ *Powered by %s*", targetURL, ctx.GetBotName())
	return ctx.ReplyWithImage(imgData, mimeType, caption)
}

func fetchWebsiteScreenshot(ctx context.Context, targetURL string) ([]byte, string, error) {
	// Source 1: Thum.io
	thumURL := "https://image.thum.io/get/width/1280/crop/800/noanimate/" + targetURL
	data, mime, err := fetchImageBytes(ctx, thumURL)
	if err == nil && len(data) > 5000 {
		return data, mime, nil
	}

	// Source 2: Microlink screenshot API
	microApiURL := "https://api.microlink.io/?url=" + url.QueryEscape(targetURL) + "&screenshot=true"
	req, err := http.NewRequestWithContext(ctx, "GET", microApiURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		resp, err := ssHttpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			var res struct {
				Status string `json:"status"`
				Data   struct {
					Screenshot struct {
						URL string `json:"url"`
					} `json:"screenshot"`
				} `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.Data.Screenshot.URL != "" {
				sData, sMime, sErr := fetchImageBytes(ctx, res.Data.Screenshot.URL)
				if sErr == nil && len(sData) > 5000 {
					return sData, sMime, nil
				}
			}
		}
	}

	// Source 3: Thum.io fullpage fallback
	thumAltURL := "https://image.thum.io/get/width/1280/noanimate/" + targetURL
	dataAlt, mimeAlt, errAlt := fetchImageBytes(ctx, thumAltURL)
	if errAlt == nil && len(dataAlt) > 5000 {
		return dataAlt, mimeAlt, nil
	}

	return nil, "", fmt.Errorf("failed to capture screenshot from available services")
}

func fetchImageBytes(ctx context.Context, imgURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", imgURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := ssHttpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	mimeType := http.DetectContentType(data)
	if !strings.HasPrefix(mimeType, "image/") {
		return nil, "", fmt.Errorf("invalid image content type: %s", mimeType)
	}

	return data, mimeType, nil
}
