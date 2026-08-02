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
	"sync"
	"time"
)

var (
	vkVideoRegex  = regexp.MustCompile(`video(-?\d+)_(\d+)`)
	vkResolutions = []string{"2160", "1440", "1080", "720", "480", "360", "240", "144"}
)

type vkAuthResponse struct {
	Response struct {
		Token     string `json:"token"`
		ExpiredAt int64  `json:"expired_at"`
	} `json:"response"`
}

type vkVideoGetResponse struct {
	Response struct {
		Items []struct {
			Title    string                 `json:"title"`
			Duration int                    `json:"duration"`
			Files    map[string]interface{} `json:"files"`
		} `json:"items"`
	} `json:"response"`
}

var (
	vkCachedToken string
	vkTokenExpiry int64
	vkTokenMu     sync.RWMutex
)

// DownloadVK extracts video from VK (vk.com / vk.ru) links.
func (c *Client) DownloadVK(ctx context.Context, rawURL string) (*Result, error) {
	resolvedURL, err := c.resolveRedirect(ctx, rawURL)
	if err != nil {
		resolvedURL = rawURL
	}

	ownerID, videoID := extractVKVideoIDs(resolvedURL)
	if ownerID == "" || videoID == "" {
		return nil, fmt.Errorf("could not parse VK video ID from URL: %s", rawURL)
	}

	token, err := c.getVKAnonymToken(ctx)
	if err != nil || token == "" {
		return nil, fmt.Errorf("failed to obtain VK anonymous token: %v", err)
	}

	apiURL := "https://api.vkvideo.ru/method/video.get"
	data := url.Values{}
	data.Set("anonymous_token", token)
	data.Set("v", "5.274")
	data.Set("lang", "en")
	data.Set("videos", fmt.Sprintf("%s_%s", ownerID, videoID))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "com.vk.vkvideo.prod/1955 (iPhone, iOS 16.7.15)")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vk video.get request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vk API HTTP status %d", resp.StatusCode)
	}

	var vgResp vkVideoGetResponse
	if err := json.NewDecoder(resp.Body).Decode(&vgResp); err != nil || len(vgResp.Response.Items) == 0 {
		return nil, fmt.Errorf("invalid VK video.get response")
	}

	item := vgResp.Response.Items[0]
	if item.Files == nil {
		return nil, fmt.Errorf("no media files available for VK video")
	}

	var videoURL string
	for _, res := range vkResolutions {
		key := "mp4_" + res
		if val, ok := item.Files[key].(string); ok && val != "" {
			videoURL = val
			break
		}
	}

	if videoURL == "" {
		return nil, fmt.Errorf("no direct MP4 stream URL found in VK video payload")
	}

	return &Result{
		Service: "vk",
		ID:      fmt.Sprintf("%s_%s", ownerID, videoID),
		Title:   item.Title,
		Items: []MediaItem{
			{
				URL:      videoURL,
				Type:     "video",
				Filename: fmt.Sprintf("vk_%s_%s.mp4", ownerID, videoID),
			},
		},
	}, nil
}

func (c *Client) getVKAnonymToken(ctx context.Context) (string, error) {
	vkTokenMu.RLock()
	if vkCachedToken != "" && time.Now().Unix() < vkTokenExpiry-10 {
		t := vkCachedToken
		vkTokenMu.RUnlock()
		return t, nil
	}
	vkTokenMu.RUnlock()

	authURL := "https://api.vk.ru/method/auth.getAnonymToken?client_id=51552953&client_secret=qgr0yWwXCrsxA1jnRtRX&v=5.274"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "com.vk.vkvideo.prod/1955 (iPhone, iOS 16.7.15)")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var auth vkAuthResponse
	if err := json.Unmarshal(body, &auth); err == nil && auth.Response.Token != "" {
		vkTokenMu.Lock()
		vkCachedToken = auth.Response.Token
		vkTokenExpiry = auth.Response.ExpiredAt
		vkTokenMu.Unlock()
		return auth.Response.Token, nil
	}

	return "", fmt.Errorf("failed to parse VK anonym token response")
}

func extractVKVideoIDs(rawURL string) (string, string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}
	m := vkVideoRegex.FindStringSubmatch(u.Path + "?" + u.RawQuery)
	if len(m) > 2 {
		return m[1], m[2]
	}
	return "", ""
}
