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
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

var ytIDRegex = regexp.MustCompile(`(?:v=|/v/|embed/|shorts/|youtu\.be/)([a-zA-Z0-9_-]{11})`)

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
		Jar:     jar,
		Timeout: 30 * time.Second,
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
	req.Header.Set("User-Agent", DefaultUserAgent)

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

type MediaInfo struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Uploader   string  `json:"uploader"`
	Duration   float64 `json:"duration"`
	Thumbnail  string  `json:"thumbnail"`
	WebpageURL string  `json:"webpage_url"`
}

type SearchResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"webpage_url"`
}

func (c *Client) runYtDlp(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("yt-dlp error: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

func (c *Client) Search(ctx context.Context, query string, limit int, provider string) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	if provider == "" {
		provider = "ytsearch"
	}

	searchTerm := fmt.Sprintf("%s%d:%s", provider, limit, query)

	out, err := c.runYtDlp(ctx, "-J", "--flat-playlist", "--no-warnings", searchTerm)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Entries []SearchResult `json:"entries"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	return payload.Entries, nil
}

func (c *Client) Info(ctx context.Context, mediaURL string) (*MediaInfo, error) {
	out, err := c.runYtDlp(ctx, "-J", "--no-warnings", "--no-playlist", mediaURL)
	if err != nil {
		return nil, err
	}

	var info MediaInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("failed to parse media info: %w", err)
	}

	return &info, nil
}

func (c *Client) DownloadYouTube(ctx context.Context, rawURL string) (*Result, error) {
	return c.DownloadYouTubeMedia(ctx, rawURL, false)
}

func (c *Client) DownloadYouTubeMedia(ctx context.Context, rawURL string, isAudioOnly bool) (*Result, error) {
	videoID := extractYouTubeID(rawURL)

	if res, err := c.fetchYtDlpURL(ctx, rawURL, videoID, isAudioOnly); err == nil && res != nil && len(res.Items) > 0 {
		return res, nil
	}

	if res, err := c.fetchEmbersDownr(ctx, rawURL, videoID, 0); err == nil && res != nil && len(res.Items) > 0 {
		return res, nil
	}

	if res, err := c.fetchVKRYouTube(ctx, rawURL, videoID); err == nil && res != nil && len(res.Items) > 0 {
		return res, nil
	}

	if res, err := c.fetchCobaltYouTube(ctx, rawURL, videoID); err == nil && res != nil && len(res.Items) > 0 {
		return res, nil
	}

	return nil, fmt.Errorf("failed to extract YouTube media for video ID %s", videoID)
}

func (c *Client) fetchYtDlpURL(ctx context.Context, rawURL, videoID string, isAudioOnly bool) (*Result, error) {
	formatSpec := "best[ext=mp4]/18/best"
	mediaType := "video"
	ext := "mp4"

	if isAudioOnly {
		formatSpec = "bestaudio[ext=m4a]/bestaudio/best"
		mediaType = "audio"
		ext = "m4a"
	}

	outURLBytes, err := c.runYtDlp(ctx, "-g", "--no-warnings", "-f", formatSpec, rawURL)
	if err != nil || len(outURLBytes) == 0 {
		return nil, fmt.Errorf("yt-dlp -g error: %v", err)
	}

	directURL := strings.TrimSpace(string(outURLBytes))
	lines := strings.Split(directURL, "\n")
	if len(lines) > 0 {
		directURL = strings.TrimSpace(lines[0])
	}

	if directURL == "" {
		return nil, fmt.Errorf("empty stream URL from yt-dlp")
	}

	title := fmt.Sprintf("YouTube %s (%s)", mediaType, videoID)

	return &Result{
		Service: "youtube",
		ID:      videoID,
		Title:   title,
		Items: []MediaItem{
			{
				URL:      directURL,
				Type:     mediaType,
				Filename: fmt.Sprintf("youtube_%s.%s", videoID, ext),
			},
		},
	}, nil
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://downr.org/.netlify/functions/bbc", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", DefaultUserAgent)
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

	client := c.getHTTPClient()
	resp, err := client.Do(req)
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

	client := c.getHTTPClient()
	resp, err := client.Do(req)
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

func (c *Client) getHTTPClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
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
