// YouTube downloader extractor derived from embers engine & downr/cobalt APIs.
package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

var (
	ytIDRegex = regexp.MustCompile(`(?:v=|/v/|embed/|shorts/|youtu\.be/)([a-zA-Z0-9_-]{11})`)
)

type embersCache struct {
	mu     sync.Mutex
	jar    *cookiejar.Jar
	client *http.Client
	ready  bool
}

var globalEmbersCache = &embersCache{}

func (ec *embersCache) getClient() (*http.Client, error) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if ec.client != nil && ec.ready {
		return ec.client, nil
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	ec.jar = jar
	ec.client = &http.Client{
		Jar: jar,
	}
	ec.ready = false
	return ec.client, nil
}

func (ec *embersCache) initSession(ctx context.Context) error {
	client, err := ec.getClient()
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://downr.org/.netlify/functions/analytics", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:152.0) Gecko/20100101 Firefox/152.0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	ec.mu.Lock()
	ec.ready = true
	ec.mu.Unlock()
	return nil
}

func (ec *embersCache) invalidate() {
	ec.mu.Lock()
	ec.ready = false
	ec.mu.Unlock()
}

type embersDownrResponse struct {
	Status   string `json:"status"`
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Text     string `json:"text"`
	Picker   []struct {
		URL  string `json:"url"`
		Type string `json:"type"`
	} `json:"picker"`
}

// DownloadYouTube extracts media items from a YouTube video URL.
func (c *Client) DownloadYouTube(ctx context.Context, rawURL string) (*Result, error) {
	videoID := extractYouTubeID(rawURL)

	// Strategy 1: Embers Downr engine
	if res, err := c.fetchEmbersDownr(ctx, rawURL, videoID, 0); err == nil && res != nil && len(res.Items) > 0 {
		return res, nil
	}

	// Strategy 2: VKR YouTube API fallback
	if res, err := c.fetchVKRYouTube(ctx, rawURL, videoID); err == nil && res != nil && len(res.Items) > 0 {
		return res, nil
	}

	// Strategy 3: Cobalt fallback API
	if res, err := c.fetchCobaltYouTube(ctx, rawURL, videoID); err == nil && res != nil && len(res.Items) > 0 {
		return res, nil
	}

	return nil, fmt.Errorf("failed to extract YouTube media for video ID %s", videoID)
}

func (c *Client) fetchEmbersDownr(ctx context.Context, mediaURL, videoID string, attempt int) (*Result, error) {
	if attempt > 2 {
		return nil, fmt.Errorf("max retries exceeded for embers downr")
	}

	if err := globalEmbersCache.initSession(ctx); err != nil {
		return nil, err
	}

	client, err := globalEmbersCache.getClient()
	if err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(map[string]string{"url": mediaURL})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://downr.org/.netlify/functions/nyt", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:152.0) Gecko/20100101 Firefox/152.0")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://downr.org/")
	req.Header.Set("Origin", "https://downr.org")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	bodyStr := string(body)
	if resp.StatusCode == http.StatusForbidden || bodyStr == "user_retry_required" || bodyStr == "action_forbidden" {
		globalEmbersCache.invalidate()
		return c.fetchEmbersDownr(ctx, mediaURL, videoID, attempt+1)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downr status %d: %s", resp.StatusCode, bodyStr)
	}

	var dRes embersDownrResponse
	if err := json.Unmarshal(body, &dRes); err != nil {
		return nil, err
	}

	var items []MediaItem
	if dRes.URL != "" {
		mType := "video"
		if dRes.Status == "audio" || strings.HasSuffix(dRes.URL, ".mp3") || strings.HasSuffix(dRes.URL, ".m4a") {
			mType = "audio"
		}
		fname := dRes.Filename
		if fname == "" {
			fname = fmt.Sprintf("youtube_%s.mp4", videoID)
		}
		items = append(items, MediaItem{
			URL:      dRes.URL,
			Type:     mType,
			Filename: fname,
		})
	}

	for i, pItem := range dRes.Picker {
		if pItem.URL != "" {
			mType := "video"
			if pItem.Type == "audio" || strings.HasSuffix(pItem.URL, ".mp3") || strings.HasSuffix(pItem.URL, ".m4a") {
				mType = "audio"
			}
			items = append(items, MediaItem{
				URL:      pItem.URL,
				Type:     mType,
				Filename: fmt.Sprintf("youtube_%s_%d.mp4", videoID, i+1),
			})
		}
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no media items found in downr response")
	}

	return &Result{
		Service: "youtube",
		ID:      videoID,
		Title:   dRes.Text,
		Items:   items,
	}, nil
}

type vkrYouTubeResponse struct {
	Status bool `json:"status"`
	Data   struct {
		Title     string `json:"title"`
		Thumbnail string `json:"thumbnail"`
		Formats   []struct {
			URL    string `json:"url"`
			Format string `json:"format_id"`
			Ext    string `json:"ext"`
			ACodec string `json:"acodec"`
			VCodec string `json:"vcodec"`
		} `json:"formats"`
		Downloads []struct {
			URL     string `json:"url"`
			Quality string `json:"quality"`
			Format  string `json:"format"`
		} `json:"downloads"`
	} `json:"data"`
}

func (c *Client) fetchVKRYouTube(ctx context.Context, mediaURL, videoID string) (*Result, error) {
	apiURL := "https://api.vkrdown.com/v1/youtube?url=" + url.QueryEscape(mediaURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", DefaultUserAgent)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vkr API status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var res vkrYouTubeResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}

	var items []MediaItem
	for _, dl := range res.Data.Downloads {
		if dl.URL != "" {
			items = append(items, MediaItem{
				URL:      dl.URL,
				Type:     "video",
				Filename: fmt.Sprintf("youtube_%s.mp4", videoID),
			})
			break
		}
	}

	if len(items) == 0 {
		for _, fmtItem := range res.Data.Formats {
			if fmtItem.URL != "" && fmtItem.VCodec != "none" {
				items = append(items, MediaItem{
					URL:      fmtItem.URL,
					Type:     "video",
					Filename: fmt.Sprintf("youtube_%s.mp4", videoID),
				})
				break
			}
		}
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no video formats in vkr response")
	}

	return &Result{
		Service: "youtube",
		ID:      videoID,
		Title:   res.Data.Title,
		Items:   items,
	}, nil
}

func (c *Client) fetchCobaltYouTube(ctx context.Context, mediaURL, videoID string) (*Result, error) {
	apiURL := "https://co.wuk.sh/api/json"
	payload, _ := json.Marshal(map[string]interface{}{
		"url": mediaURL,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cobalt status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var res embersDownrResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}

	if res.URL == "" && len(res.Picker) == 0 {
		return nil, fmt.Errorf("no media URL in cobalt response")
	}

	var items []MediaItem
	if res.URL != "" {
		items = append(items, MediaItem{
			URL:      res.URL,
			Type:     "video",
			Filename: fmt.Sprintf("youtube_%s.mp4", videoID),
		})
	}

	return &Result{
		Service: "youtube",
		ID:      videoID,
		Title:   res.Text,
		Items:   items,
	}, nil
}

func extractYouTubeID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	if u.Host == "youtu.be" {
		return strings.TrimPrefix(u.Path, "/")
	}

	m := ytIDRegex.FindStringSubmatch(rawURL)
	if len(m) > 1 {
		return m[1]
	}

	return u.Query().Get("v")
}
