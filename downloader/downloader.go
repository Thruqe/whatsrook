package downloader

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"whatsrook/logger"
)

var mainLog = logger.WhatsmeowStyle("Downloader/Router", "DEBUG", true)

const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

type MediaItem struct {
	URL      string `json:"url"`
	Type     string `json:"type"` // "video", "photo", "audio", "gif"
	Filename string `json:"filename,omitempty"`
	ThumbURL string `json:"thumb_url,omitempty"`
	Buffer   []byte `json:"-"`
}

type Result struct {
	Service     string      `json:"service"`
	ID          string      `json:"id,omitempty"`
	Title       string      `json:"title,omitempty"`
	Author      string      `json:"author,omitempty"`
	Items       []MediaItem `json:"items"`
	IsPhoto     bool        `json:"is_photo,omitempty"`
	IsAudioOnly bool        `json:"is_audio_only,omitempty"`
}

type Client struct {
	HTTPClient *http.Client
}

func NewClient() *Client {
	return &Client{
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

var defaultClient = NewClient()

func Download(ctx context.Context, rawURL string) (*Result, error) {
	return defaultClient.Download(ctx, rawURL)
}

func (c *Client) Download(ctx context.Context, rawURL string) (*Result, error) {
	mainLog.Debugf("Routing download request for URL: %s", rawURL)

	if err := ValidateURL(rawURL); err != nil {
		mainLog.Warnf("URL validation failed: %v", err)
		return nil, err
	}

	cleanURL := strings.TrimSpace(rawURL)
	u, err := url.Parse(cleanURL)
	if err != nil {
		mainLog.Errorf("Failed to parse URL string: %v", err)
		return nil, err
	}

	host := strings.ToLower(u.Host)
	mainLog.Debugf("Extracted URL host: %s", host)

	switch {
	case strings.Contains(host, "facebook.com") || strings.Contains(host, "fb.watch") || strings.Contains(host, "fb.gg") || strings.Contains(host, "fb.com"):
		mainLog.Infof("Selected Facebook handler for host: %s", host)
		return c.DownloadFacebook(ctx, cleanURL)

	case strings.Contains(host, "instagram.com") || strings.Contains(host, "instagr.am"):
		mainLog.Infof("Selected Instagram handler for host: %s", host)
		return c.DownloadInstagram(ctx, cleanURL)

	case strings.Contains(host, "snapchat.com") || strings.Contains(host, "t.snapchat.com"):
		mainLog.Infof("Selected Snapchat handler for host: %s", host)
		return c.DownloadSnapchat(ctx, cleanURL)

	case strings.Contains(host, "twitter.com") || strings.Contains(host, "x.com") || strings.Contains(host, "twimg.com") || host == "t.co" || strings.HasSuffix(host, ".t.co"):
		mainLog.Infof("Selected Twitter handler for host: %s", host)
		return c.DownloadTwitter(ctx, cleanURL)

	case strings.Contains(host, "tiktok.com") || strings.Contains(host, "vt.tiktok.com") || strings.Contains(host, "vm.tiktok.com"):
		mainLog.Infof("Selected TikTok handler for host: %s", host)
		return c.DownloadTikTok(ctx, cleanURL)

	case strings.Contains(host, "reddit.com") || strings.Contains(host, "v.redd.it"):
		mainLog.Infof("Selected Reddit handler for host: %s", host)
		return c.DownloadReddit(ctx, cleanURL)

	case strings.Contains(host, "pinterest.com") || strings.Contains(host, "pinterest.ca") || strings.Contains(host, "pin.it"):
		mainLog.Infof("Selected Pinterest handler for host: %s", host)
		return c.DownloadPinterest(ctx, cleanURL)

	case strings.Contains(host, "soundcloud.com") || strings.Contains(host, "on.soundcloud.com"):
		mainLog.Infof("Selected SoundCloud handler for host: %s", host)
		return c.DownloadSoundCloud(ctx, cleanURL)

	case strings.Contains(host, "threads.net") || strings.Contains(host, "threads.com"):
		mainLog.Infof("Selected Threads handler for host: %s", host)
		return c.DownloadThreads(ctx, cleanURL)

	case strings.Contains(host, "bsky.app") || strings.Contains(host, "bsky.social"):
		mainLog.Infof("Selected Bluesky handler for host: %s", host)
		return c.DownloadBluesky(ctx, cleanURL)

	case strings.Contains(host, "vk.com") || strings.Contains(host, "vk.ru") || strings.Contains(host, "vkvideo.ru"):
		mainLog.Infof("Selected VK handler for host: %s", host)
		return c.DownloadVK(ctx, cleanURL)

	case strings.Contains(host, "tumblr.com"):
		mainLog.Infof("Selected Tumblr handler for host: %s", host)
		return c.DownloadTumblr(ctx, cleanURL)

	case strings.Contains(host, "youtube.com") || strings.Contains(host, "youtu.be"):
		mainLog.Infof("Selected YouTube handler for host: %s", host)
		return c.DownloadYouTube(ctx, cleanURL)

	case strings.Contains(host, "twitch.tv") || strings.Contains(host, "clips.twitch.tv"):
		mainLog.Infof("Selected Twitch handler for host: %s", host)
		return c.DownloadTwitch(ctx, cleanURL)

	default:
		mainLog.Errorf("No handler matching host: %s", host)
		return nil, fmt.Errorf("unsupported URL or domain: %s", host)
	}
}

func DownloadInstagramStories(ctx context.Context, username string) (*Result, error) {
	return defaultClient.InstagramStories(ctx, username)
}

func IsValidURL(rawURL string) bool {
	return ValidateURL(rawURL) == nil
}

func ValidateURL(rawURL string) error {
	clean := strings.TrimSpace(rawURL)
	if clean == "" {
		return fmt.Errorf("empty URL provided")
	}
	if !strings.HasPrefix(clean, "http://") && !strings.HasPrefix(clean, "https://") {
		return fmt.Errorf("URL must start with http:// or https://")
	}
	u, err := url.ParseRequestURI(clean)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid URL structure: %v", err)
	}
	host := strings.ToLower(u.Host)
	if !isSupportedHost(host) {
		return fmt.Errorf("unsupported social media domain: %s", host)
	}
	return nil
}

func isSupportedHost(host string) bool {
	switch {
	case strings.Contains(host, "snapchat.com"), strings.Contains(host, "t.snapchat.com"),
		strings.Contains(host, "youtube.com"), strings.Contains(host, "youtu.be"),
		strings.Contains(host, "facebook.com"), strings.Contains(host, "fb.watch"), strings.Contains(host, "fb.gg"), strings.Contains(host, "fb.com"),
		strings.Contains(host, "instagram.com"), strings.Contains(host, "instagr.am"),
		strings.Contains(host, "twitter.com"), strings.Contains(host, "x.com"), strings.Contains(host, "twimg.com"), host == "t.co", strings.HasSuffix(host, ".t.co"),
		strings.Contains(host, "tiktok.com"), strings.Contains(host, "vt.tiktok.com"), strings.Contains(host, "vm.tiktok.com"),
		strings.Contains(host, "reddit.com"), strings.Contains(host, "v.redd.it"),
		strings.Contains(host, "pinterest.com"), strings.Contains(host, "pinterest.ca"), strings.Contains(host, "pin.it"),
		strings.Contains(host, "soundcloud.com"), strings.Contains(host, "on.soundcloud.com"),
		strings.Contains(host, "threads.net"), strings.Contains(host, "threads.com"),
		strings.Contains(host, "bsky.app"), strings.Contains(host, "bsky.social"),
		strings.Contains(host, "vk.com"), strings.Contains(host, "vk.ru"), strings.Contains(host, "vkvideo.ru"),
		strings.Contains(host, "tumblr.com"),
		strings.Contains(host, "twitch.tv"), strings.Contains(host, "clips.twitch.tv"):
		return true
	default:
		return false
	}
}
