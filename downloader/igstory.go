package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"whatsrook/logger"
)

const (
	baseURL   = "https://clipssaver.com"
	userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"
)

var igLog = logger.WhatsmeowStyle("Downloader/IG", "INFO", true)

type StoriesResponse struct {
	Status string `json:"status"`
	Data   struct {
		Stories []struct {
			ID          int64   `json:"id"`
			DisplayURL  string  `json:"displayUrl"`
			DownloadURL string  `json:"downloadUrl"`
			VideoUrl    *string `json:"videoUrl"`
			IsVideo     bool    `json:"isVideo"`
			TakenAt     int64   `json:"takenAt"`
		} `json:"stories"`
	} `json:"data"`
}

// InstagramStories fetches stories for a given username and returns structured results.
func (c *Client) InstagramStories(ctx context.Context, username string) (*Result, error) {
	username = strings.TrimPrefix(strings.TrimSpace(username), "@")
	if username == "" {
		return nil, fmt.Errorf("invalid username supplied")
	}

	igLog.Infof("Fetching Instagram stories for user: %s", username)

	payload, err := json.Marshal(map[string]string{"username": username})
	if err != nil {
		igLog.Errorf("Failed to marshal request payload: %v", err)
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/instagram/instagramDownloader/stories", bytes.NewBuffer(payload))
	if err != nil {
		igLog.Errorf("Failed to construct HTTP request: %v", err)
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", baseURL)
	req.Header.Set("Referer", baseURL+"/instagram-story-downloader")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		igLog.Errorf("HTTP request execution failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		igLog.Errorf("Upstream service returned status: %s", resp.Status)
		return nil, fmt.Errorf("HTTP Error: %s", resp.Status)
	}

	var storiesRes StoriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&storiesRes); err != nil {
		igLog.Errorf("Failed to parse JSON response: %v", err)
		return nil, err
	}

	var items []MediaItem
	for i, story := range storiesRes.Data.Stories {
		mediaType := "photo"
		mediaURL := story.DownloadURL
		ext := "jpg"

		if story.IsVideo && story.VideoUrl != nil && *story.VideoUrl != "" {
			mediaType = "video"
			mediaURL = *story.VideoUrl
			ext = "mp4"
		}

		if mediaURL == "" {
			mediaURL = story.DisplayURL
		}

		items = append(items, MediaItem{
			URL:      mediaURL,
			Type:     mediaType,
			Filename: fmt.Sprintf("%s_story_%d.%s", username, i+1, ext),
		})
	}

	igLog.Infof("Extracted %d story items for user %s", len(items), username)

	return &Result{
		Service: "instagram",
		Author:  username,
		Title:   fmt.Sprintf("Instagram Stories for @%s", username),
		Items:   items,
	}, nil
}
