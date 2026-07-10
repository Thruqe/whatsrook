package ember

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const baseURL = "https://embers-uk0r.onrender.com/download"

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

type Result struct {
	Error    bool   `json:"error"`
	ErrorMsg string `json:"message,omitempty"`
	Data     Data   `json:"data"`
}

type Data struct {
	URL       string  `json:"url"`
	Source    string  `json:"source"`
	Title     string  `json:"title"`
	Author    string  `json:"author"`
	Thumbnail string  `json:"thumbnail"`
	Owner     Owner   `json:"owner"`
	Type      string  `json:"type"` // "single" or "multiple"
	Medias    []Media `json:"medias"`
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Fetch calls the Ember API for the given post/video URL.
func Fetch(ctx context.Context, postURL string) (*Data, error) {
	q := url.Values{"url": {postURL}}
	fullURL := baseURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ember request failed: %w", err)
	}
	defer resp.Body.Close()

	var result Result
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ember decode failed: %w", err)
	}

	if result.Error {
		msg := result.ErrorMsg
		if msg == "" {
			msg = "unknown error from ember api"
		}
		return nil, fmt.Errorf("ember: %s", msg)
	}

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
