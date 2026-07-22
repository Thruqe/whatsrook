package ember

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const baseURL string = "https://embers-0kn7.onrender.com/download"

type Owner struct {
	Username string `json:"username"`
	FullName string `json:"full_name"`
}

type Media struct {
	URL       string `json:"url"`
	Type      string `json:"type"` // "video", "image", "audio"
	Extension string `json:"extension"`
	IsAudio   bool   `json:"is_audio"`
}

type FormatInfo struct {
	FormatID   *string `json:"format_id"`
	URL        *string `json:"url"`
	Ext        *string `json:"ext"`
	Resolution any     `json:"resolution"`
	Filesize   any     `json:"filesize"`
	VCodec     *string `json:"vcodec"`
	ACodec     *string `json:"acodec"`
	FPS        any     `json:"fps"`
}

type Result struct {
	Error    bool   `json:"error"`
	ErrorMsg string `json:"message,omitempty"`
	Data     Data   `json:"data"`
}

type Data struct {
	ID           *string        `json:"id"`
	RawTitle     *string        `json:"title"`
	Description  *string        `json:"description"`
	Duration     any            `json:"duration"`
	RawThumbnail *string        `json:"thumbnail"`
	Thumbnails   any            `json:"thumbnails"`
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

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Fetch calls the Ember API for the given post/video URL with an optional cookie.
func Fetch(ctx context.Context, postURL string, cookie string) (*Data, error) {
	q := url.Values{"url": {postURL}}
	if cookie != "" {
		q.Set("cookie", cookie)
	}
	fullURL := baseURL + "?" + q.Encode()

	slog.Info("ember.Fetch: sending HTTP request", "url", fullURL)
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

	slog.Info("ember.Fetch: HTTP response received", "status_code", resp.StatusCode)
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
		slog.Error("ember.Fetch: API returned error", "msg", msg)
		return nil, fmt.Errorf("ember: %s", msg)
	}

	result.Data.PopulateCompat()
	slog.Info("ember.Fetch: successfully populated compat data", "title", result.Data.Title, "medias_count", len(result.Data.Medias))

	return &result.Data, nil
}

// BestMedia picks the primary video/image to send, skipping audio-only tracks.
func (d *Data) BestMedia() (*Media, bool) {
	for _, m := range d.Medias {
		if m.Type == "video" || m.Type == "image" {
			return &m, true
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
	if len(d.Medias) > 1 {
		d.Type = "multiple"
	} else {
		d.Type = "single"
	}
	if len(d.Medias) > 0 {
		d.URL = d.Medias[0].URL
	}
}

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

	// Otherwise, extract from the main Formats list
	var bestVideoAndAudio *FormatInfo
	var bestVideoOnly *FormatInfo
	var bestAudioOnly *FormatInfo

	for i := range d.Formats {
		f := &d.Formats[i]
		if f.URL == nil || *f.URL == "" {
			continue
		}

		// Skip HLS/m3u8 playlists
		extVal := ""
		if f.Ext != nil {
			extVal = *f.Ext
		}
		if extVal == "m3u8" || strings.Contains(strings.ToLower(*f.URL), ".m3u8") {
			continue
		}

		vcodec := ""
		if f.VCodec != nil {
			vcodec = *f.VCodec
		}
		acodec := ""
		if f.ACodec != nil {
			acodec = *f.ACodec
		}

		hasVideo := vcodec != "" && vcodec != "none"
		hasAudio := acodec != "" && acodec != "none"

		if hasVideo && hasAudio {
			bestVideoAndAudio = f
		} else if hasVideo {
			bestVideoOnly = f
		} else if hasAudio {
			bestAudioOnly = f
		}
	}

	var mediaURL string
	var mediaType string
	var ext string

	if bestVideoAndAudio != nil {
		mediaURL = *bestVideoAndAudio.URL
		mediaType = "video"
		if bestVideoAndAudio.Ext != nil {
			ext = *bestVideoAndAudio.Ext
		}
	} else if bestVideoOnly != nil {
		mediaURL = *bestVideoOnly.URL
		mediaType = "video"
		if bestVideoOnly.Ext != nil {
			ext = *bestVideoOnly.Ext
		}
	} else if bestAudioOnly != nil {
		mediaURL = *bestAudioOnly.URL
		mediaType = "audio"
		if bestAudioOnly.Ext != nil {
			ext = *bestAudioOnly.Ext
		}
	} else {
		// Fallback to top-level fields
		// Check raw top-level URL
		if d.Raw != nil {
			if topURL, ok := d.Raw["url"].(string); ok && topURL != "" {
				if !strings.Contains(strings.ToLower(topURL), ".m3u8") {
					mediaURL = topURL
					if topExt, ok := d.Raw["ext"].(string); ok {
						ext = topExt
					}
				}
			}
		}
		if mediaURL == "" && d.RawThumbnail != nil && *d.RawThumbnail != "" {
			mediaURL = *d.RawThumbnail
			mediaType = "image"
		}

		if mediaURL != "" {
			if ext == "" {
				if u, err := url.Parse(mediaURL); err == nil {
					ext = filepath.Ext(u.Path)
				}
			}
			ext = strings.TrimPrefix(ext, ".")
			switch strings.ToLower(ext) {
			case "jpg", "jpeg", "png", "webp", "gif":
				mediaType = "image"
			case "mp3", "m4a", "ogg", "opus", "wav":
				mediaType = "audio"
			default:
				mediaType = "video"
			}
		}
	}

	if mediaURL != "" {
		return []Media{
			{
				URL:       mediaURL,
				Type:      mediaType,
				Extension: ext,
				IsAudio:   mediaType == "audio",
			},
		}
	}

	return nil
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

	// Determine if there are formats
	var formats []any
	if fmts, ok := info["formats"].([]any); ok {
		formats = fmts
	}

	var bestVideoAndAudio map[string]any
	var bestVideoOnly map[string]any
	var bestAudioOnly map[string]any

	for _, fVal := range formats {
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

		vcodec, _ := f["vcodec"].(string)
		acodec, _ := f["acodec"].(string)

		hasVideo := vcodec != "" && vcodec != "none"
		hasAudio := acodec != "" && acodec != "none"

		if hasVideo && hasAudio {
			bestVideoAndAudio = f
		} else if hasVideo {
			bestVideoOnly = f
		} else if hasAudio {
			bestAudioOnly = f
		}
	}

	var mediaURL string
	var mediaType string
	var ext string

	if bestVideoAndAudio != nil {
		mediaURL, _ = bestVideoAndAudio["url"].(string)
		mediaType = "video"
		ext, _ = bestVideoAndAudio["ext"].(string)
	} else if bestVideoOnly != nil {
		mediaURL, _ = bestVideoOnly["url"].(string)
		mediaType = "video"
		ext, _ = bestVideoOnly["ext"].(string)
	} else if bestAudioOnly != nil {
		mediaURL, _ = bestAudioOnly["url"].(string)
		mediaType = "audio"
		ext, _ = bestAudioOnly["ext"].(string)
	} else {
		// Fallback to top-level URL
		if topURL, ok := info["url"].(string); ok && topURL != "" {
			mediaURL = topURL
			ext, _ = info["ext"].(string)
			if ext == "" {
				if u, err := url.Parse(topURL); err == nil {
					ext = filepath.Ext(u.Path)
				}
			}
			ext = strings.TrimPrefix(ext, ".")
			switch strings.ToLower(ext) {
			case "jpg", "jpeg", "png", "webp", "gif":
				mediaType = "image"
			case "mp3", "m4a", "ogg", "opus", "wav":
				mediaType = "audio"
			default:
				mediaType = "video"
			}
		}
	}

	if mediaURL != "" {
		return []Media{
			{
				URL:       mediaURL,
				Type:      mediaType,
				Extension: ext,
				IsAudio:   mediaType == "audio",
			},
		}
	}

	return nil
}
