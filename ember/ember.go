// Ember media download service – downloads media from supported platforms
package ember

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	baseURL       = "https://embers-0kn7.onrender.com/download"
	baseSearchURL = "https://embers-0kn7.onrender.com/youtube/search"
)

// Owner holds information about the content creator of downloaded media.
type Owner struct {
	Username string `json:"username"`
	FullName string `json:"full_name"`
}

// Media represents a single downloadable media item from a platform.
type Media struct {
	URL       string `json:"url"`
	Type      string `json:"type"` // "video", "image", "audio"
	Extension string `json:"extension"`
	IsAudio   bool   `json:"is_audio"`
}

// FormatInfo describes a single downloadable format variant (resolution, codec, etc.).
type FormatInfo struct {
	FormatID   *string `json:"format_id"`
	URL        *string `json:"url"`
	Ext        *string `json:"ext"`
	Resolution any     `json:"resolution"`
	Filesize   any     `json:"filesize"`
	VCodec     *string `json:"vcodec"`
	ACodec     *string `json:"acodec"`
	FPS        any     `json:"fps"`
	Protocol   *string `json:"protocol"`
	Width      *int    `json:"width"`
	Height     *int    `json:"height"`
}

// Result wraps the API response, including error status and downloaded data.
type Result struct {
	Error    bool   `json:"error"`
	ErrorMsg string `json:"message,omitempty"`
	Data     Data   `json:"data"`
}

// Thumbnail represents a single thumbnail variant.
type Thumbnail struct {
	ID         string `json:"id"`
	URL        string `json:"url"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Resolution string `json:"resolution"`
}

// Data holds the full download result including metadata, owner info, and media items.
type Data struct {
	ID           *string        `json:"id"`
	RawTitle     *string        `json:"title"`
	Description  *string        `json:"description"`
	Duration     any            `json:"duration"`
	RawThumbnail *string        `json:"thumbnail"`
	Thumbnails   []Thumbnail    `json:"thumbnails"`
	Uploader     *string        `json:"uploader"`
	UploaderURL  *string        `json:"uploader_url"`
	WebpageURL   *string        `json:"webpage_url"`
	Extractor    *string        `json:"extractor"`
	Formats      []FormatInfo   `json:"formats"`
	Raw          map[string]any `json:"raw"`

	// Derived fields for backward compatibility
	URL       string  `json:"-"`
	Source    string  `json:"-"`
	Title     string  `json:"-"`
	Author    string  `json:"-"`
	Thumbnail string  `json:"-"`
	Owner     Owner   `json:"-"`
	Type      string  `json:"-"`
	Medias    []Media `json:"-"`
}

var httpClient = &http.Client{Timeout: 90 * time.Second}

// SearchResult is a single entry returned by the Ember /youtube/search endpoint.
type SearchResult struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	URL       string  `json:"url"`
	Duration  float64 `json:"duration,omitempty"`
	ViewCount float64 `json:"view_count,omitempty"`
	Uploader  string  `json:"uploader,omitempty"`
	Thumbnail string  `json:"thumbnail,omitempty"`
}

// searchResponse is the envelope returned by /youtube/search.
type searchResponse struct {
	Error   bool   `json:"error"`
	Message string `json:"message,omitempty"`
	Data    struct {
		Query   string         `json:"query"`
		Count   int            `json:"count"`
		Results []SearchResult `json:"results"`
	} `json:"data"`
}

// SearchYouTube calls the Ember /youtube/search endpoint and returns the top results.
func SearchYouTube(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 1
	}
	q := url.Values{
		"q":     {query},
		"limit": {fmt.Sprintf("%d", limit)},
	}
	fullURL := baseSearchURL + "?" + q.Encode()

	slog.Debug("ember.SearchYouTube: sending request", "url", fullURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ember search request failed: %w", err)
	}
	defer resp.Body.Close()

	var sr searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("ember search decode failed: %w", err)
	}
	if sr.Error {
		return nil, fmt.Errorf("ember search: %s", sr.Message)
	}
	return sr.Data.Results, nil
}

// Fetch calls the Ember /download API for the given URL.
// Cookies are managed server-side via POST /cookies — no cookie param needed here.
func Fetch(ctx context.Context, postURL string, _ string) (*Data, error) {
	q := url.Values{"url": {postURL}}
	fullURL := baseURL + "?" + q.Encode()

	slog.Debug("ember.Fetch: sending HTTP request", "url", fullURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		slog.Error("ember.Fetch: failed to create request", "err", err)
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		slog.Error("ember.Fetch: httpClient.Do failed", "err", err)
		return nil, fmt.Errorf("ember request failed: %w", err)
	}
	defer resp.Body.Close()

	slog.Debug("ember.Fetch: HTTP response received", "status_code", resp.StatusCode)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest &&
		resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusInternalServerError {
		return nil, fmt.Errorf("ember API returned status %d", resp.StatusCode)
	}

	var result Result
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		slog.Error("ember.Fetch: failed to decode JSON", "err", err)
		return nil, fmt.Errorf("ember decode failed: %w", err)
	}

	if result.Error {
		msg := result.ErrorMsg
		if msg == "" {
			msg = "unknown error from ember api"
		}
		slog.Warn("ember.Fetch: API returned error", "msg", msg)
		return nil, fmt.Errorf("ember: %s", msg)
	}

	result.Data.PopulateCompat()
	slog.Debug("ember.Fetch: successfully populated compat data",
		"title", result.Data.Title,
		"medias_count", len(result.Data.Medias))

	return &result.Data, nil
}

// BestMedia picks the primary video/image to send, skipping audio-only tracks.
func (d *Data) BestMedia() (*Media, bool) {
	for i := range d.Medias {
		m := &d.Medias[i]
		if m.Type == "video" || m.Type == "image" {
			return m, true
		}
	}
	if len(d.Medias) > 0 {
		return &d.Medias[0], true
	}
	return nil, false
}

// Caption builds the text sent alongside the media.
func (d *Data) Caption() string {
	if d.Title == "" && d.Author == "" {
		return ""
	}
	if d.Author != "" {
		return fmt.Sprintf("%s\n— %s", d.Title, d.Author)
	}
	return d.Title
}

// PopulateCompat populates fields needed for backward compatibility.
func (d *Data) PopulateCompat() {
	if d.RawTitle != nil {
		d.Title = *d.RawTitle
	}
	if d.RawThumbnail != nil {
		d.Thumbnail = *d.RawThumbnail
	}
	if d.WebpageURL != nil {
		d.Source = *d.WebpageURL
	}
	if d.Uploader != nil {
		d.Author = *d.Uploader
	}

	d.Medias = extractMediasFromData(d)
	switch len(d.Medias) {
	case 0:
		d.Type = "none"
	case 1:
		d.Type = "single"
	default:
		d.Type = "multiple"
	}
	if len(d.Medias) > 0 {
		d.URL = d.Medias[0].URL
	}
}

// --- format scoring ---

// formatScore returns a score for a format. Higher is better.
// We prefer: direct download > HLS, video+audio > video-only > audio-only,
// larger filesize, higher resolution.
func formatScore(f *FormatInfo) int {
	score := 0

	// Direct downloads (https/http) are preferred over HLS/m3u8
	if f.Protocol != nil {
		proto := strings.ToLower(*f.Protocol)
		if proto == "https" || proto == "http" {
			score += 1000
		} else if strings.Contains(proto, "m3u8") {
			score -= 500
		}
	}

	// Video + audio is best
	hasVideo := hasVideo(f)
	hasAudio := hasAudio(f)
	if hasVideo && hasAudio {
		score += 500
	} else if hasVideo {
		score += 300
	} else if hasAudio {
		score += 100
	}

	// Prefer larger filesize
	if fs := filesizeBytes(f.Filesize); fs > 0 {
		score += int(fs / (1024 * 1024)) // +1 per MB, capped by int range
	}

	// Prefer higher resolution (width * height)
	w, h := resolutionDims(f)
	if w > 0 && h > 0 {
		score += w * h / 10000
	}

	return score
}

func hasVideo(f *FormatInfo) bool {
	if f.VCodec != nil && *f.VCodec != "" && *f.VCodec != "none" {
		return true
	}
	// Fallback: if resolution looks like dimensions and not "audio only"
	if res, ok := f.Resolution.(string); ok && res != "" && res != "audio only" {
		return strings.Contains(res, "x")
	}
	// Fallback: has width and height
	return f.Width != nil && *f.Width > 0 && f.Height != nil && *f.Height > 0
}

func hasAudio(f *FormatInfo) bool {
	if f.ACodec != nil && *f.ACodec != "" && *f.ACodec != "none" {
		return true
	}
	// Fallback: resolution says "audio only"
	if res, ok := f.Resolution.(string); ok && res == "audio only" {
		return true
	}
	return false
}

func filesizeBytes(v any) int64 {
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int:
		return int64(val)
	case int64:
		return val
	case string:
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			return n
		}
	}
	return 0
}

func resolutionDims(f *FormatInfo) (int, int) {
	if f.Width != nil && f.Height != nil {
		return *f.Width, *f.Height
	}
	if res, ok := f.Resolution.(string); ok {
		parts := strings.Split(res, "x")
		if len(parts) == 2 {
			w, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
			h, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
			return w, h
		}
	}
	return 0, 0
}

// isHLS returns true if the format is an HLS/m3u8 stream.
func isHLS(f *FormatInfo) bool {
	if f.Ext != nil && *f.Ext == "m3u8" {
		return true
	}
	if f.URL != nil && strings.Contains(strings.ToLower(*f.URL), ".m3u8") {
		return true
	}
	if f.Protocol != nil && strings.Contains(strings.ToLower(*f.Protocol), "m3u8") {
		return true
	}
	return false
}

// --- media extraction ---

func extractMediasFromData(d *Data) []Media {
	if d == nil {
		return nil
	}

	// First check if raw entries exist for carousel/playlist
	if d.Raw != nil {
		if entriesVal, ok := d.Raw["entries"]; ok && entriesVal != nil {
			if entries, ok := entriesVal.([]any); ok {
				var list []Media
				for _, entryVal := range entries {
					if entry, ok := entryVal.(map[string]any); ok {
						list = append(list, extractMediasFromMap(entry)...)
					}
				}
				if len(list) > 0 {
					return list
				}
			}
		}
	}

	// Build formats from typed data
	var candidates []formatCandidate
	for i := range d.Formats {
		f := &d.Formats[i]
		if f.URL == nil || *f.URL == "" {
			continue
		}
		if isHLS(f) {
			continue
		}
		candidates = append(candidates, formatCandidate{
			url:      *f.URL,
			score:    formatScore(f),
			hasVideo: hasVideo(f),
			hasAudio: hasAudio(f),
			ext:      stringOrEmpty(f.Ext),
		})
	}

	if len(candidates) > 0 {
		// Sort by score descending
		for i := 0; i < len(candidates)-1; i++ {
			for j := i + 1; j < len(candidates); j++ {
				if candidates[j].score > candidates[i].score {
					candidates[i], candidates[j] = candidates[j], candidates[i]
				}
			}
		}

		// Pick the best candidate
		best := candidates[0]
		mediaType := classifyMedia(best.ext, best.hasVideo, best.hasAudio)
		return []Media{{
			URL:       best.url,
			Type:      mediaType,
			Extension: best.ext,
			IsAudio:   mediaType == "audio",
		}}
	}

	// Fallback to raw top-level URL
	if d.Raw != nil {
		if topURL, ok := d.Raw["url"].(string); ok && topURL != "" {
			if !strings.Contains(strings.ToLower(topURL), ".m3u8") {
				ext := ""
				if topExt, ok := d.Raw["ext"].(string); ok {
					ext = topExt
				}
				if ext == "" {
					if u, err := url.Parse(topURL); err == nil {
						ext = filepath.Ext(u.Path)
					}
				}
				ext = strings.TrimPrefix(ext, ".")
				mediaType := classifyMedia(ext, false, false)
				return []Media{{
					URL:       topURL,
					Type:      mediaType,
					Extension: ext,
					IsAudio:   mediaType == "audio",
				}}
			}
		}
	}

	// Fallback to thumbnail as image
	if d.RawThumbnail != nil && *d.RawThumbnail != "" {
		return []Media{{
			URL:       *d.RawThumbnail,
			Type:      "image",
			Extension: "jpg",
			IsAudio:   false,
		}}
	}

	return nil
}

type formatCandidate struct {
	url      string
	score    int
	hasVideo bool
	hasAudio bool
	ext      string
}

func extractMediasFromMap(info map[string]any) []Media {
	if info == nil {
		return nil
	}

	// Handle playlist/carousel entries
	if entriesVal, ok := info["entries"]; ok && entriesVal != nil {
		if entries, ok := entriesVal.([]any); ok {
			var list []Media
			for _, entryVal := range entries {
				if entry, ok := entryVal.(map[string]any); ok {
					list = append(list, extractMediasFromMap(entry)...)
				}
			}
			if len(list) > 0 {
				return list
			}
		}
	}

	// Build formats from untyped map
	var candidates []formatCandidate

	if fmts, ok := info["formats"].([]any); ok {
		for _, fVal := range fmts {
			f, ok := fVal.(map[string]any)
			if !ok {
				continue
			}
			fURL, _ := f["url"].(string)
			if fURL == "" {
				continue
			}

			// Skip HLS/m3u8 playlists
			extVal, _ := f["ext"].(string)
			if extVal == "m3u8" || strings.Contains(strings.ToLower(fURL), ".m3u8") {
				continue
			}
			protoVal, _ := f["protocol"].(string)
			if strings.Contains(strings.ToLower(protoVal), "m3u8") {
				continue
			}

			vcodec, _ := f["vcodec"].(string)
			acodec, _ := f["acodec"].(string)
			resolution, _ := f["resolution"].(string)
			width, _ := f["width"].(float64)
			height, _ := f["height"].(float64)

			hasVid := vcodec != "" && vcodec != "none"
			hasAud := acodec != "" && acodec != "none"

			// Fallback detection for null codecs
			if !hasVid && !hasAud {
				if resolution != "" && resolution != "audio only" && strings.Contains(resolution, "x") {
					hasVid = true
				}
				if resolution == "audio only" {
					hasAud = true
				}
				if width > 0 && height > 0 {
					hasVid = true
				}
			}

			score := 0
			proto := strings.ToLower(protoVal)
			if proto == "https" || proto == "http" {
				score += 1000
			} else if strings.Contains(proto, "m3u8") {
				score -= 500
			}
			if hasVid && hasAud {
				score += 500
			} else if hasVid {
				score += 300
			} else if hasAud {
				score += 100
			}

			// filesize
			var fs int64
			switch v := f["filesize"].(type) {
			case float64:
				fs = int64(v)
			case int:
				fs = int64(v)
			case string:
				if n, err := strconv.ParseInt(v, 10, 64); err == nil {
					fs = n
				}
			}
			score += int(fs / (1024 * 1024))

			// resolution
			if w, h := parseResolution(resolution); w > 0 && h > 0 {
				score += w * h / 10000
			}
			if width > 0 && height > 0 {
				score += int(width*height) / 10000
			}

			candidates = append(candidates, formatCandidate{
				url:      fURL,
				score:    score,
				hasVideo: hasVid,
				hasAudio: hasAud,
				ext:      extVal,
			})
		}
	}

	if len(candidates) > 0 {
		// Sort by score descending (simple bubble sort for clarity)
		for i := 0; i < len(candidates)-1; i++ {
			for j := i + 1; j < len(candidates); j++ {
				if candidates[j].score > candidates[i].score {
					candidates[i], candidates[j] = candidates[j], candidates[i]
				}
			}
		}

		best := candidates[0]
		mediaType := classifyMedia(best.ext, best.hasVideo, best.hasAudio)
		return []Media{{
			URL:       best.url,
			Type:      mediaType,
			Extension: best.ext,
			IsAudio:   mediaType == "audio",
		}}
	}

	// Fallback to top-level URL
	if topURL, ok := info["url"].(string); ok && topURL != "" {
		ext, _ := info["ext"].(string)
		if ext == "" {
			if u, err := url.Parse(topURL); err == nil {
				ext = filepath.Ext(u.Path)
			}
		}
		ext = strings.TrimPrefix(ext, ".")
		mediaType := classifyMedia(ext, false, false)
		return []Media{{
			URL:       topURL,
			Type:      mediaType,
			Extension: ext,
			IsAudio:   mediaType == "audio",
		}}
	}

	// Fallback to thumbnail
	if thumb, ok := info["thumbnail"].(string); ok && thumb != "" {
		return []Media{{
			URL:       thumb,
			Type:      "image",
			Extension: "jpg",
			IsAudio:   false,
		}}
	}

	return nil
}

// --- helpers ---

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func parseResolution(res string) (int, int) {
	parts := strings.Split(res, "x")
	if len(parts) != 2 {
		return 0, 0
	}
	w, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	return w, h
}

func classifyMedia(ext string, hasVideo, hasAudio bool) string {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))

	// Explicit type based on codec info
	if hasVideo {
		return "video"
	}
	if hasAudio && !hasVideo {
		return "audio"
	}

	// Fallback based on extension
	switch ext {
	case "jpg", "jpeg", "png", "webp", "gif", "bmp", "tiff":
		return "image"
	case "mp3", "m4a", "ogg", "opus", "wav", "flac", "aac":
		return "audio"
	case "mp4", "webm", "mov", "mkv", "avi", "flv":
		return "video"
	default:
		return "video" // safest default
	}
}
