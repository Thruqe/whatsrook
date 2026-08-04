package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"whatsrook/logger"
)

var snapLog = logger.WhatsmeowStyle("Downloader/Snapchat", "DEBUG", true)

var (
	snapSpotlightRegex = regexp.MustCompile(`<link[^>]+rel="preload"[^>]+href="([^"]+)"[^>]+as="video"`)
	snapVideoSrcRegex  = regexp.MustCompile(`<video[^>]+src="([^"]+)"`)
	snapMediaURLRegex  = regexp.MustCompile(`"mediaUrl":"([^"]+)"`)
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
	snapLog.Debugf("Starting Snapchat download pipeline for raw URL: %s", rawURL)

	resolvedURL, err := c.resolveRedirect(ctx, rawURL)
	if err != nil {
		snapLog.Warnf("Failed to resolve redirects, proceeding with raw URL: %v", err)
		resolvedURL = rawURL
	} else {
		snapLog.Debugf("Resolved target URL: %s", resolvedURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolvedURL, nil)
	if err != nil {
		snapLog.Errorf("Failed to construct HTTP request: %v", err)
		return nil, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	snapLog.Debugf("Executing GET request to Snapchat...")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		snapLog.Errorf("Snapchat HTTP request failed: %v", err)
		return nil, fmt.Errorf("snapchat request failed: %w", err)
	}
	defer resp.Body.Close()

	snapLog.Debugf("Received HTTP response status: %s (%d)", resp.Status, resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		snapLog.Errorf("Non-200 HTTP response received: %d", resp.StatusCode)
		return nil, fmt.Errorf("snapchat HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		snapLog.Errorf("Failed to read response body: %v", err)
		return nil, err
	}

	html := string(body)
	snapLog.Debugf("Successfully fetched HTML payload (size: %d bytes)", len(html))

	// Stage 1: Preload video tag search
	snapLog.Debugf("Attempting Stage 1: Preload video link extraction...")
	if m := snapSpotlightRegex.FindStringSubmatch(html); len(m) > 1 {
		videoURL := strings.ReplaceAll(m[1], "&amp;", "&")
		snapLog.Infof("Stage 1 SUCCESS: Extracted video preload URL: %s", videoURL)
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
	snapLog.Debugf("Stage 1 failed: No preload video link found.")

	// Stage 2: Direct video tag search
	snapLog.Debugf("Attempting Stage 2: Direct <video src> tag extraction...")
	if m := snapVideoSrcRegex.FindStringSubmatch(html); len(m) > 1 {
		videoURL := strings.ReplaceAll(m[1], "&amp;", "&")
		snapLog.Infof("Stage 2 SUCCESS: Extracted video tag URL: %s", videoURL)
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
	snapLog.Debugf("Stage 2 failed: No video src tag found.")

	// Stage 3: Direct JSON string attribute search
	snapLog.Debugf("Attempting Stage 3: Direct \"mediaUrl\" JSON regex matching...")
	if m := snapMediaURLRegex.FindStringSubmatch(html); len(m) > 1 {
		mediaURL := strings.ReplaceAll(m[1], `\u0026`, "&")
		mediaURL = strings.ReplaceAll(mediaURL, "&amp;", "&")
		snapLog.Infof("Stage 3 SUCCESS: Extracted JSON mediaUrl: %s", mediaURL)
		return &Result{
			Service: "snapchat",
			Items: []MediaItem{
				{
					URL:      mediaURL,
					Type:     "video",
					Filename: "snapchat_spotlight.mp4",
				},
			},
		}, nil
	}
	snapLog.Debugf("Stage 3 failed: No inline mediaUrl JSON field found.")

	// Stage 4: Next.js script element extraction
	snapLog.Debugf("Attempting Stage 4: __NEXT_DATA__ JSON script tag parsing...")
	if m := snapNextDataRegex.FindStringSubmatch(html); len(m) > 1 {
		snapLog.Debugf("Found __NEXT_DATA__ script block (length: %d bytes), unmarshaling...", len(m[1]))
		var nextData snapNextData
		if err := json.Unmarshal([]byte(m[1]), &nextData); err == nil {
			var items []MediaItem
			if len(nextData.Props.PageProps.CuratedHighlights) > 0 {
				snaps := nextData.Props.PageProps.CuratedHighlights[0].SnapList
				snapLog.Debugf("Parsed CuratedHighlights with %d snaps", len(snaps))
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
				snapLog.Infof("Stage 4 SUCCESS: Extracted %d story items from __NEXT_DATA__", len(items))
				return &Result{
					Service: "snapchat",
					Items:   items,
				}, nil
			}
		} else {
			snapLog.Warnf("Failed to unmarshal __NEXT_DATA__ JSON: %v", err)
		}
	}
	snapLog.Debugf("Stage 4 failed: No valid media extracted from __NEXT_DATA__.")

	snapLog.Errorf("All Snapchat extraction stages failed for URL: %s", resolvedURL)
	return nil, fmt.Errorf("no downloadable media found in Snapchat page")
}

func (c *Client) resolveRedirect(ctx context.Context, rawURL string) (string, error) {
	snapLog.Debugf("Resolving HTTP redirects for: %s", rawURL)
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
			snapLog.Debugf("Followed redirect [%d]: %s", len(via), finalURL)
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
