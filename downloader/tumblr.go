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

var tumblrPostRegex = regexp.MustCompile(`/post/(\d+)`)

type tumblrResponse struct {
	Response struct {
		Timeline struct {
			Elements []struct {
				Content []struct {
					Type    string `json:"type"`
					Provider string `json:"provider"`
					Title   string `json:"title"`
					Artist  string `json:"artist"`
					Media   *struct {
						URL string `json:"url"`
					} `json:"media"`
				} `json:"content"`
			} `json:"elements"`
		} `json:"timeline"`
	} `json:"response"`
}

// DownloadTumblr extracts video or audio from a Tumblr post link.
func (c *Client) DownloadTumblr(ctx context.Context, rawURL string) (*Result, error) {
	resolvedURL, err := c.resolveRedirect(ctx, rawURL)
	if err != nil {
		resolvedURL = rawURL
	}

	domain, postID := extractTumblrDomainAndID(resolvedURL)
	if postID == "" {
		return nil, fmt.Errorf("could not parse Tumblr post ID from URL: %s", rawURL)
	}

	apiURL := fmt.Sprintf("https://api-http2.tumblr.com/v2/blog/%s/posts/%s/permalink?api_key=jrsCWX1XDuVxAFO4GkK147syAoN8BJZ5voz8tS80bPcj26Vc5Z", domain, postID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Tumblr/iPhone/33.3/333010/17.3.1/tumblr")
	req.Header.Set("X-Version", "iPhone/33.3/333010/17.3.1/tumblr")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tumblr request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tumblr HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data tumblrResponse
	if err := json.Unmarshal(body, &data); err != nil || len(data.Response.Timeline.Elements) == 0 {
		return nil, fmt.Errorf("invalid tumblr API response payload")
	}

	contents := data.Response.Timeline.Elements[0].Content
	for _, item := range contents {
		if item.Media != nil && item.Media.URL != "" {
			switch item.Type {
			case "video":
				return &Result{
					Service: "tumblr",
					ID:      postID,
					Items: []MediaItem{
						{
							URL:      item.Media.URL,
							Type:     "video",
							Filename: fmt.Sprintf("tumblr_%s.mp4", postID),
						},
					},
				}, nil
			case "audio":
				return &Result{
					Service:     "tumblr",
					ID:          postID,
					Title:       item.Title,
					Author:      item.Artist,
					IsAudioOnly: true,
					Items: []MediaItem{
						{
							URL:      item.Media.URL,
							Type:     "audio",
							Filename: fmt.Sprintf("tumblr_%s.mp3", postID),
						},
					},
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("no video or audio media found in Tumblr post %s", postID)
}

func extractTumblrDomainAndID(rawURL string) (string, string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}

	postID := ""
	m := tumblrPostRegex.FindStringSubmatch(u.Path)
	if len(m) > 1 {
		postID = m[1]
	}

	domain := u.Host
	if domain == "www.tumblr.com" || domain == "tumblr.com" {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 2 {
			domain = fmt.Sprintf("%s.tumblr.com", parts[0])
		}
	}

	return domain, postID
}
