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

var redditCommentsRegex = regexp.MustCompile(`/comments/([a-zA-Z0-9]+)`)

type redditPostResponse []struct {
	Data struct {
		Children []struct {
			Data struct {
				Title       string `json:"title"`
				Subreddit   string `json:"subreddit"`
				URL         string `json:"url"`
				SecureMedia *struct {
					RedditVideo *struct {
						FallbackURL string `json:"fallback_url"`
						HLSURL      string `json:"hls_url"`
					} `json:"reddit_video"`
				} `json:"secure_media"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

// DownloadReddit extracts media (videos, gifs) from Reddit posts or v.redd.it links.
func (c *Client) DownloadReddit(ctx context.Context, rawURL string) (*Result, error) {
	resolvedURL, err := c.resolveRedirect(ctx, rawURL)
	if err != nil {
		resolvedURL = rawURL
	}

	postID := extractRedditPostID(resolvedURL)
	if postID == "" {
		return nil, fmt.Errorf("could not extract Reddit post ID from URL: %s", rawURL)
	}

	apiURL := fmt.Sprintf("https://www.reddit.com/comments/%s.json", postID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reddit API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reddit HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data redditPostResponse
	if err := json.Unmarshal(body, &data); err != nil || len(data) == 0 || len(data[0].Data.Children) == 0 {
		return nil, fmt.Errorf("invalid reddit API JSON response")
	}

	itemData := data[0].Data.Children[0].Data
	title := itemData.Title

	// 1. Direct GIF
	if strings.HasSuffix(strings.ToLower(itemData.URL), ".gif") {
		return &Result{
			Service: "reddit",
			ID:      postID,
			Title:   title,
			Author:  itemData.Subreddit,
			Items: []MediaItem{
				{
					URL:      itemData.URL,
					Type:     "gif",
					Filename: fmt.Sprintf("reddit_%s.gif", postID),
				},
			},
		}, nil
	}

	// 2. Reddit Video
	if itemData.SecureMedia != nil && itemData.SecureMedia.RedditVideo != nil {
		fallbackURL := itemData.SecureMedia.RedditVideo.FallbackURL
		cleanURL := strings.Split(fallbackURL, "?")[0]

		return &Result{
			Service: "reddit",
			ID:      postID,
			Title:   title,
			Author:  itemData.Subreddit,
			Items: []MediaItem{
				{
					URL:      cleanURL,
					Type:     "video",
					Filename: fmt.Sprintf("reddit_%s.mp4", postID),
				},
			},
		}, nil
	}

	// 3. Direct Image Fallback
	if strings.HasSuffix(strings.ToLower(itemData.URL), ".jpg") || strings.HasSuffix(strings.ToLower(itemData.URL), ".jpeg") || strings.HasSuffix(strings.ToLower(itemData.URL), ".png") {
		return &Result{
			Service: "reddit",
			ID:      postID,
			Title:   title,
			Author:  itemData.Subreddit,
			IsPhoto: true,
			Items: []MediaItem{
				{
					URL:      itemData.URL,
					Type:     "photo",
					Filename: fmt.Sprintf("reddit_%s.jpg", postID),
				},
			},
		}, nil
	}

	return nil, fmt.Errorf("no video or supported media found in Reddit post")
}

func extractRedditPostID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	m := redditCommentsRegex.FindStringSubmatch(u.Path)
	if len(m) > 1 {
		return m[1]
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 && (parts[0] == "video" || parts[0] == "v.redd.it") {
		return parts[1]
	}
	return ""
}
