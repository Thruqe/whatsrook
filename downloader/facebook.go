package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var (
	fbHDRegex    = regexp.MustCompile(`"browser_native_hd_url":(".*?")`)
	fbSDRegex    = regexp.MustCompile(`"browser_native_sd_url":(".*?")`)
	fbPlayableHD = regexp.MustCompile(`"playable_url_quality_hd":"([^"]+)"`)
	fbPlayableSD = regexp.MustCompile(`"playable_url":"([^"]+)"`)
	fbSrcHDRegex = regexp.MustCompile(`hd_src:"([^"]+)"`)
	fbSrcSDRegex = regexp.MustCompile(`sd_src:"([^"]+)"`)
)

// DownloadFacebook extracts media from a Facebook video, reel, or share link.
func (c *Client) DownloadFacebook(ctx context.Context, rawURL string) (*Result, error) {
	resolvedURL, err := c.resolveFacebookURL(ctx, rawURL)
	if err != nil {
		resolvedURL = rawURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolvedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("facebook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("facebook HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	html := string(body)
	videoURL := extractFacebookVideoURL(html)
	if videoURL == "" {
		return nil, fmt.Errorf("failed to extract video URL from Facebook page")
	}

	videoID := extractFacebookID(resolvedURL)
	filename := fmt.Sprintf("facebook_%s.mp4", videoID)

	return &Result{
		Service: "facebook",
		ID:      videoID,
		Items: []MediaItem{
			{
				URL:      videoURL,
				Type:     "video",
				Filename: filename,
			},
		},
	}, nil
}

func (c *Client) resolveFacebookURL(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return rawURL, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	// Custom check redirect
	var finalURL string
	client := &http.Client{
		Timeout: c.HTTPClient.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			finalURL = req.URL.String()
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		if finalURL != "" {
			return finalURL, nil
		}
		return rawURL, err
	}
	defer resp.Body.Close()

	if finalURL != "" {
		return finalURL, nil
	}
	return resp.Request.URL.String(), nil
}

func extractFacebookVideoURL(html string) string {
	var candidates []string

	if m := fbHDRegex.FindStringSubmatch(html); len(m) > 1 {
		var decoded string
		if err := json.Unmarshal([]byte(m[1]), &decoded); err == nil && decoded != "" {
			candidates = append(candidates, decoded)
		}
	}
	if m := fbSDRegex.FindStringSubmatch(html); len(m) > 1 {
		var decoded string
		if err := json.Unmarshal([]byte(m[1]), &decoded); err == nil && decoded != "" {
			candidates = append(candidates, decoded)
		}
	}
	if m := fbPlayableHD.FindStringSubmatch(html); len(m) > 1 {
		candidates = append(candidates, cleanEscapedURL(m[1]))
	}
	if m := fbSrcHDRegex.FindStringSubmatch(html); len(m) > 1 {
		candidates = append(candidates, cleanEscapedURL(m[1]))
	}
	if m := fbPlayableSD.FindStringSubmatch(html); len(m) > 1 {
		candidates = append(candidates, cleanEscapedURL(m[1]))
	}
	if m := fbSrcSDRegex.FindStringSubmatch(html); len(m) > 1 {
		candidates = append(candidates, cleanEscapedURL(m[1]))
	}

	for _, urlStr := range candidates {
		if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
			return urlStr
		}
	}
	return ""
}

func cleanEscapedURL(raw string) string {
	clean := strings.ReplaceAll(raw, `\`, "")
	clean = strings.ReplaceAll(clean, `\u0026`, "&")
	clean = strings.ReplaceAll(clean, `&amp;`, "&")
	return clean
}

func extractFacebookID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "video"
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		if last != "" {
			return last
		}
	}
	if v := u.Query().Get("v"); v != "" {
		return v
	}
	return "video"
}
