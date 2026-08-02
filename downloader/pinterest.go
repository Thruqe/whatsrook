package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var (
	pinVideoRegex = regexp.MustCompile(`"url":"(https://v1\.pinimg\.com/videos/.*?)"`)
	pinImageRegex = regexp.MustCompile(`src="(https://i\.pinimg\.com/.*\.(?:jpg|gif))"`)
	pinIDRegex    = regexp.MustCompile(`/pin/([^/?#]+)`)
)

// DownloadPinterest extracts videos, gifs, or photos from a Pinterest pin.
func (c *Client) DownloadPinterest(ctx context.Context, rawURL string) (*Result, error) {
	resolvedURL, err := c.resolveRedirect(ctx, rawURL)
	if err != nil {
		resolvedURL = rawURL
	}

	pinID := extractPinterestID(resolvedURL)
	if pinID == "" {
		return nil, fmt.Errorf("could not extract Pinterest Pin ID from URL: %s", rawURL)
	}

	targetURL := fmt.Sprintf("https://www.pinterest.com/pin/%s/", pinID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pinterest request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pinterest HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)

	// 1. Video Link
	videoMatches := pinVideoRegex.FindAllStringSubmatch(html, -1)
	for _, m := range videoMatches {
		if len(m) > 1 && strings.HasSuffix(m[1], ".mp4") {
			videoURL := cleanEscapedURL(m[1])
			return &Result{
				Service: "pinterest",
				ID:      pinID,
				Items: []MediaItem{
					{
						URL:      videoURL,
						Type:     "video",
						Filename: fmt.Sprintf("pinterest_%s.mp4", pinID),
					},
				},
			}, nil
		}
	}

	// 2. Image Link
	imageMatches := pinImageRegex.FindAllStringSubmatch(html, -1)
	for _, m := range imageMatches {
		if len(m) > 1 {
			imageURL := cleanEscapedURL(m[1])
			itemType := "photo"
			ext := "jpg"
			if strings.HasSuffix(imageURL, ".gif") {
				itemType = "gif"
				ext = "gif"
			}
			return &Result{
				Service: "pinterest",
				ID:      pinID,
				IsPhoto: itemType == "photo",
				Items: []MediaItem{
					{
						URL:      imageURL,
						Type:     itemType,
						Filename: fmt.Sprintf("pinterest_%s.%s", pinID, ext),
					},
				},
			}, nil
		}
	}

	return nil, fmt.Errorf("no media found in Pinterest Pin %s", pinID)
}

func extractPinterestID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	m := pinIDRegex.FindStringSubmatch(u.Path)
	if len(m) > 1 {
		id := m[1]
		if strings.Contains(id, "--") {
			parts := strings.Split(id, "--")
			id = parts[len(parts)-1]
		}
		return id
	}
	return ""
}
