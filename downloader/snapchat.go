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
	snapSpotlightRegex = regexp.MustCompile(`<link[^>]+rel="preload"[^>]+href="([^"]+)"[^>]+as="video"`)
	snapNextDataRegex  = regexp.MustCompile(`<script id="__NEXT_DATA__" type="application/json">(.*?)</script>`)
)

type snapNextData struct {
	Props struct {
		PageProps struct {
			CuratedHighlights []struct {
				SnapList []struct {
					SnapMediaType int `json:"snapMediaType"`
					SnapUrls      struct {
						MediaUrl        string `json:"mediaUrl"`
						MediaPreviewUrl struct {
							Value string `json:"value"`
						} `json:"mediaPreviewUrl"`
					} `json:"snapUrls"`
				} `json:"snapList"`
			} `json:"curatedHighlights"`
		} `json:"pageProps"`
	} `json:"props"`
}

// DownloadSnapchat extracts media from Snapchat Spotlight or Story links.
func (c *Client) DownloadSnapchat(ctx context.Context, rawURL string) (*Result, error) {
	resolvedURL, err := c.resolveRedirect(ctx, rawURL)
	if err != nil {
		resolvedURL = rawURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolvedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("snapchat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("snapchat HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)

	// Spotlight Video Regex
	if m := snapSpotlightRegex.FindStringSubmatch(html); len(m) > 1 {
		videoURL := m[1]
		return &Result{
			Service: "snapchat",
			Items: []MediaItem{
				{
					URL:      videoURL,
					Type:     "video",
					Filename: "snapchat_spotlight.mp4",
				},
			},
		}, nil
	}

	// Next.js Data Extraction for Stories / Highlights
	if m := snapNextDataRegex.FindStringSubmatch(html); len(m) > 1 {
		var nextData snapNextData
		if json.Unmarshal([]byte(m[1]), &nextData) == nil {
			var items []MediaItem
			if len(nextData.Props.PageProps.CuratedHighlights) > 0 {
				snaps := nextData.Props.PageProps.CuratedHighlights[0].SnapList
				for i, snap := range snaps {
					if snap.SnapUrls.MediaUrl != "" {
						mediaType := "photo"
						ext := "jpg"
						if snap.SnapMediaType != 0 {
							mediaType = "video"
							ext = "mp4"
						}
						items = append(items, MediaItem{
							URL:      snap.SnapUrls.MediaUrl,
							Type:     mediaType,
							Filename: fmt.Sprintf("snapchat_story_%d.%s", i+1, ext),
							ThumbURL: snap.SnapUrls.MediaPreviewUrl.Value,
						})
					}
				}
			}
			if len(items) > 0 {
				return &Result{
					Service: "snapchat",
					Items:   items,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("no downloadable media found in Snapchat page")
}

func (c *Client) resolveRedirect(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return rawURL, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

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

func extractSnapchatUsername(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "add" {
		return parts[1]
	}
	return ""
}
