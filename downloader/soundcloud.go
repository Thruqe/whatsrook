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
)

var (
	scClientIDRegex = regexp.MustCompile(`client_id:"([A-Za-z0-9]{32})"`)
	scScriptSrcRegex = regexp.MustCompile(`<script[^>]+src="([^"]+)"`)
)

type scResolveResponse struct {
	ID                 int    `json:"id"`
	Title              string `json:"title"`
	TrackAuthorization string `json:"track_authorization"`
	User               struct {
		Username string `json:"username"`
	} `json:"user"`
	ArtworkURL string `json:"artwork_url"`
	Media      struct {
		Transcodings []struct {
			URL    string `json:"url"`
			Format struct {
				Protocol string `json:"protocol"`
				MimeType string `json:"mime_type"`
			} `json:"format"`
			Preset string `json:"preset"`
		} `json:"transcodings"`
	} `json:"media"`
}

type scStreamResponse struct {
	URL string `json:"url"`
}

var (
	cachedSCClientID string
	scClientMu       sync.RWMutex
)

// DownloadSoundCloud extracts audio track stream from SoundCloud links.
func (c *Client) DownloadSoundCloud(ctx context.Context, rawURL string) (*Result, error) {
	resolvedURL, err := c.resolveRedirect(ctx, rawURL)
	if err != nil {
		resolvedURL = rawURL
	}

	clientID, err := c.getSoundCloudClientID(ctx)
	if err != nil || clientID == "" {
		return nil, fmt.Errorf("failed to fetch SoundCloud client ID: %v", err)
	}

	resolveAPI := fmt.Sprintf("https://api-v2.soundcloud.com/resolve?url=%s&client_id=%s", url.QueryEscape(resolvedURL), clientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolveAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("soundcloud resolve request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("soundcloud resolve HTTP status %d", resp.StatusCode)
	}

	var track scResolveResponse
	if err := json.NewDecoder(resp.Body).Decode(&track); err != nil {
		return nil, fmt.Errorf("failed to decode soundcloud resolve JSON: %w", err)
	}

	if len(track.Media.Transcodings) == 0 {
		return nil, fmt.Errorf("no media transcodings found for soundcloud track")
	}

	// Select best stream (progressive mp3 or opus)
	var selectedURL string
	for _, tc := range track.Media.Transcodings {
		if tc.Format.Protocol == "progressive" || strings.Contains(tc.Format.MimeType, "mpeg") {
			selectedURL = tc.URL
			break
		}
	}
	if selectedURL == "" && len(track.Media.Transcodings) > 0 {
		selectedURL = track.Media.Transcodings[0].URL
	}

	if selectedURL == "" {
		return nil, fmt.Errorf("no usable audio stream found")
	}

	streamAPI := fmt.Sprintf("%s?client_id=%s&track_authorization=%s", selectedURL, clientID, url.QueryEscape(track.TrackAuthorization))
	streamReq, err := http.NewRequestWithContext(ctx, http.MethodGet, streamAPI, nil)
	if err != nil {
		return nil, err
	}
	streamReq.Header.Set("User-Agent", DefaultUserAgent)

	streamResp, err := c.HTTPClient.Do(streamReq)
	if err != nil {
		return nil, fmt.Errorf("soundcloud stream request failed: %w", err)
	}
	defer streamResp.Body.Close()

	var streamData scStreamResponse
	if err := json.NewDecoder(streamResp.Body).Decode(&streamData); err != nil || streamData.URL == "" {
		return nil, fmt.Errorf("failed to retrieve direct soundcloud audio URL")
	}

	title := track.Title
	artist := track.User.Username
	filename := fmt.Sprintf("soundcloud_%d.mp3", track.ID)

	thumb := track.ArtworkURL
	if thumb != "" {
		thumb = strings.ReplaceAll(thumb, "-large", "-t500x500")
	}

	return &Result{
		Service:     "soundcloud",
		ID:          fmt.Sprintf("%d", track.ID),
		Title:       title,
		Author:      artist,
		IsAudioOnly: true,
		Items: []MediaItem{
			{
				URL:      streamData.URL,
				Type:     "audio",
				Filename: filename,
				ThumbURL: thumb,
			},
		},
	}, nil
}

func (c *Client) getSoundCloudClientID(ctx context.Context) (string, error) {
	scClientMu.RLock()
	if cachedSCClientID != "" {
		id := cachedSCClientID
		scClientMu.RUnlock()
		return id, nil
	}
	scClientMu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://soundcloud.com/", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	html := string(body)
	scripts := scScriptSrcRegex.FindAllStringSubmatch(html, -1)

	for _, s := range scripts {
		if len(s) > 1 && strings.HasPrefix(s[1], "https://a-v2.sndcdn.com/") {
			scrReq, errScr := http.NewRequestWithContext(ctx, http.MethodGet, s[1], nil)
			if errScr == nil {
				scrReq.Header.Set("User-Agent", DefaultUserAgent)
				scrResp, errDo := c.HTTPClient.Do(scrReq)
				if errDo == nil && scrResp.StatusCode == http.StatusOK {
					scrBody, _ := io.ReadAll(scrResp.Body)
					scrResp.Body.Close()
					if m := scClientIDRegex.FindStringSubmatch(string(scrBody)); len(m) > 1 {
						scClientMu.Lock()
						cachedSCClientID = m[1]
						scClientMu.Unlock()
						return m[1], nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("soundcloud client_id not found in scripts")
}
