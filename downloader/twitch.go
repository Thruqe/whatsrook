package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var (
	twitchClipRegex = regexp.MustCompile(`/(?:clip/|clips\.twitch\.tv/)([a-zA-Z0-9_-]+)`)
)

type twitchClipMetaResponse struct {
	Data struct {
		Clip *struct {
			Title       string `json:"title"`
			Broadcaster struct {
				Login string `json:"login"`
			} `json:"broadcaster"`
			VideoQualities []struct {
				Quality   string `json:"quality"`
				SourceURL string `json:"sourceURL"`
			} `json:"videoQualities"`
		} `json:"clip"`
	} `json:"data"`
}

type twitchTokenResponse []struct {
	Data struct {
		Clip struct {
			PlaybackAccessToken struct {
				Signature string `json:"signature"`
				Value     string `json:"value"`
			} `json:"playbackAccessToken"`
		} `json:"clip"`
	} `json:"data"`
}

// DownloadTwitch extracts MP4 video streams from Twitch Clip links.
func (c *Client) DownloadTwitch(ctx context.Context, rawURL string) (*Result, error) {
	resolvedURL, err := c.resolveRedirect(ctx, rawURL)
	if err != nil {
		resolvedURL = rawURL
	}

	slug := extractTwitchClipSlug(resolvedURL)
	if slug == "" {
		return nil, fmt.Errorf("could not extract Twitch Clip slug from URL: %s", rawURL)
	}

	gqlURL := "https://gql.twitch.tv/gql"
	clientID := "kimne78kx3ncx6brgo4mv6wki5h1ko"

	// 1. Fetch Clip Metadata
	metaQuery := fmt.Sprintf(`{"query": "{ clip(slug: \"%s\") { broadcaster { login } title videoQualities { quality sourceURL } } }" }`, slug)
	metaReq, err := http.NewRequestWithContext(ctx, http.MethodPost, gqlURL, bytes.NewBufferString(metaQuery))
	if err != nil {
		return nil, err
	}
	metaReq.Header.Set("Client-Id", clientID)
	metaReq.Header.Set("Content-Type", "application/json")

	metaResp, err := c.HTTPClient.Do(metaReq)
	if err != nil {
		return nil, fmt.Errorf("twitch GQL metadata request failed: %w", err)
	}
	defer metaResp.Body.Close()

	if metaResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitch GQL HTTP status %d", metaResp.StatusCode)
	}

	var metaData twitchClipMetaResponse
	if err := json.NewDecoder(metaResp.Body).Decode(&metaData); err != nil || metaData.Data.Clip == nil {
		return nil, fmt.Errorf("failed to parse Twitch clip metadata payload")
	}

	clip := metaData.Data.Clip
	if len(clip.VideoQualities) == 0 {
		return nil, fmt.Errorf("no video qualities found for Twitch clip %s", slug)
	}

	// 2. Fetch Playback Access Token
	tokenQuery := fmt.Sprintf(`[{"operationName":"VideoAccessToken_Clip","variables":{"slug":"%s"},"extensions":{"persistedQuery":{"version":1,"sha256Hash":"36b89d2507fce29e5ca551df756d27c1cfe079e2609642b4390aa4c35796eb11"}}}]`, slug)
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, gqlURL, bytes.NewBufferString(tokenQuery))
	if err != nil {
		return nil, err
	}
	tokenReq.Header.Set("Client-Id", clientID)
	tokenReq.Header.Set("Content-Type", "application/json")

	tokenResp, err := c.HTTPClient.Do(tokenReq)
	if err != nil {
		return nil, fmt.Errorf("twitch GQL token request failed: %w", err)
	}
	defer tokenResp.Body.Close()

	body, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		return nil, err
	}

	var tokenData twitchTokenResponse
	if err := json.Unmarshal(body, &tokenData); err != nil || len(tokenData) == 0 {
		return nil, fmt.Errorf("failed to parse Twitch playback access token")
	}

	pat := tokenData[0].Data.Clip.PlaybackAccessToken
	bestQuality := clip.VideoQualities[0]

	directURL := fmt.Sprintf("%s?sig=%s&token=%s", bestQuality.SourceURL, pat.Signature, url.QueryEscape(pat.Value))

	return &Result{
		Service: "twitch",
		ID:      slug,
		Title:   clip.Title,
		Author:  clip.Broadcaster.Login,
		Items: []MediaItem{
			{
				URL:      directURL,
				Type:     "video",
				Filename: fmt.Sprintf("twitch_%s.mp4", slug),
			},
		},
	}, nil
}

func extractTwitchClipSlug(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	m := twitchClipRegex.FindStringSubmatch(u.Host + u.Path)
	if len(m) > 1 {
		return m[1]
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 && parts[len(parts)-1] != "" {
		return parts[len(parts)-1]
	}
	return ""
}
