package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var bskyURLRegex = regexp.MustCompile(`/profile/([^/]+)/post/([^/?#]+)`)

type bskyPostThreadResponse struct {
	Thread struct {
		Post struct {
			Embed struct {
				Type     string `json:"$type"`
				Playlist string `json:"playlist"`
				Images   []struct {
					Fullsize string `json:"fullsize"`
				} `json:"images"`
				External *struct {
					URI string `json:"uri"`
				} `json:"external"`
			} `json:"embed"`
		} `json:"post"`
	} `json:"thread"`
	Error string `json:"error"`
}

// DownloadBluesky extracts media (videos, photos, gifs) from Bluesky post links.
func (c *Client) DownloadBluesky(ctx context.Context, rawURL string) (*Result, error) {
	resolvedURL, err := c.resolveRedirect(ctx, rawURL)
	if err != nil {
		resolvedURL = rawURL
	}

	user, postID := extractBlueskyUserAndPostID(resolvedURL)
	if user == "" || postID == "" {
		return nil, fmt.Errorf("could not parse Bluesky user/post ID from URL: %s", rawURL)
	}

	apiURL := fmt.Sprintf("https://public.api.bsky.app/xrpc/app.bsky.feed.getPostThread?depth=0&parentHeight=0&uri=at://%s/app.bsky.feed.post/%s", user, postID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bluesky request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bluesky HTTP status %d", resp.StatusCode)
	}

	var data bskyPostThreadResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse Bluesky JSON response: %w", err)
	}

	if data.Error != "" {
		return nil, fmt.Errorf("bluesky API error: %s", data.Error)
	}

	embed := data.Thread.Post.Embed
	var items []MediaItem
	isPhoto := true

	switch {
	case strings.Contains(embed.Type, "images") && len(embed.Images) > 0:
		for i, img := range embed.Images {
			if img.Fullsize != "" {
				items = append(items, MediaItem{
					URL:      img.Fullsize,
					Type:     "photo",
					Filename: fmt.Sprintf("bluesky_%s_%s_%d.jpg", user, postID, i+1),
				})
			}
		}

	case strings.Contains(embed.Type, "video") && embed.Playlist != "":
		isPhoto = false
		playlistURL := embed.Playlist
		if strings.Contains(playlistURL, "video.bsky.app/watch/") {
			playlistURL = strings.ReplaceAll(playlistURL, "video.bsky.app/watch/", "video.cdn.bsky.app/hls/")
		}
		items = append(items, MediaItem{
			URL:      playlistURL,
			Type:     "video",
			Filename: fmt.Sprintf("bluesky_%s_%s.mp4", user, postID),
		})

	case strings.Contains(embed.Type, "external") && embed.External != nil && embed.External.URI != "":
		extURI := embed.External.URI
		if strings.Contains(extURI, "tenor.com") || strings.HasSuffix(extURI, ".gif") {
			items = append(items, MediaItem{
				URL:      extURI,
				Type:     "gif",
				Filename: fmt.Sprintf("bluesky_%s_%s.gif", user, postID),
			})
		}
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no media items found in Bluesky post")
	}

	return &Result{
		Service: "bluesky",
		ID:      postID,
		Author:  user,
		Items:   items,
		IsPhoto: isPhoto,
	}, nil
}

func extractBlueskyUserAndPostID(rawURL string) (string, string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}
	m := bskyURLRegex.FindStringSubmatch(u.Path)
	if len(m) > 2 {
		return m[1], m[2]
	}
	return "", ""
}
