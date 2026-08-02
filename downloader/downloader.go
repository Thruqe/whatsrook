// Package downloader provides media download extractors for Facebook, Instagram,
// Twitter (X), and TikTok based on Cobalt engine service patterns.
package downloader

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// MediaItem represents a single extracted media item (video, photo, or audio).
type MediaItem struct {
	URL      string `json:"url"`
	Type     string `json:"type"` // "video", "photo", "audio", "gif"
	Filename string `json:"filename,omitempty"`
	ThumbURL string `json:"thumb_url,omitempty"`
}

// Result contains the output of a successful media extraction.
type Result struct {
	Service     string      `json:"service"` // "facebook", "instagram", "twitter", "tiktok"
	ID          string      `json:"id,omitempty"`
	Title       string      `json:"title,omitempty"`
	Author      string      `json:"author,omitempty"`
	Items       []MediaItem `json:"items"`
	IsPhoto     bool        `json:"is_photo,omitempty"`
	IsAudioOnly bool        `json:"is_audio_only,omitempty"`
}

// Client represents a downloader client with configurable HTTP transport and timeout.
type Client struct {
	HTTPClient *http.Client
}

// NewClient initializes a default downloader client.
func NewClient() *Client {
	return &Client{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

var defaultClient = NewClient()

// Download detects the target service from rawURL and executes extraction.
func Download(ctx context.Context, rawURL string) (*Result, error) {
	return defaultClient.Download(ctx, rawURL)
}

// Download detects the service type and invokes appropriate extractor.
func (c *Client) Download(ctx context.Context, rawURL string) (*Result, error) {
	cleanURL := strings.TrimSpace(rawURL)
	if cleanURL == "" {
		return nil, fmt.Errorf("empty URL provided")
	}

	u, err := url.Parse(cleanURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	host := strings.ToLower(u.Host)

	switch {
	case strings.Contains(host, "facebook.com") || strings.Contains(host, "fb.watch") || strings.Contains(host, "fb.gg") || strings.Contains(host, "fb.com"):
		return c.DownloadFacebook(ctx, cleanURL)

	case strings.Contains(host, "instagram.com") || strings.Contains(host, "instagr.am"):
		return c.DownloadInstagram(ctx, cleanURL)

	case strings.Contains(host, "twitter.com") || strings.Contains(host, "x.com") || strings.Contains(host, "twimg.com") || strings.Contains(host, "t.co"):
		return c.DownloadTwitter(ctx, cleanURL)

	case strings.Contains(host, "tiktok.com") || strings.Contains(host, "vt.tiktok.com") || strings.Contains(host, "vm.tiktok.com"):
		return c.DownloadTikTok(ctx, cleanURL)

	case strings.Contains(host, "snapchat.com") || strings.Contains(host, "t.snapchat.com"):
		return c.DownloadSnapchat(ctx, cleanURL)

	case strings.Contains(host, "reddit.com") || strings.Contains(host, "v.redd.it"):
		return c.DownloadReddit(ctx, cleanURL)

	case strings.Contains(host, "pinterest.com") || strings.Contains(host, "pinterest.ca") || strings.Contains(host, "pin.it"):
		return c.DownloadPinterest(ctx, cleanURL)

	case strings.Contains(host, "soundcloud.com") || strings.Contains(host, "on.soundcloud.com"):
		return c.DownloadSoundCloud(ctx, cleanURL)

	case strings.Contains(host, "threads.net") || strings.Contains(host, "threads.com"):
		return c.DownloadThreads(ctx, cleanURL)

	case strings.Contains(host, "bsky.app") || strings.Contains(host, "bsky.social"):
		return c.DownloadBluesky(ctx, cleanURL)

	case strings.Contains(host, "vk.com") || strings.Contains(host, "vk.ru") || strings.Contains(host, "vkvideo.ru"):
		return c.DownloadVK(ctx, cleanURL)

	case strings.Contains(host, "tumblr.com"):
		return c.DownloadTumblr(ctx, cleanURL)

	case strings.Contains(host, "twitch.tv") || strings.Contains(host, "clips.twitch.tv"):
		return c.DownloadTwitch(ctx, cleanURL)

	default:
		return nil, fmt.Errorf("unsupported URL or domain: %s", host)
	}
}
