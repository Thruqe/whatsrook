package downloader_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"whatsrook/downloader"
)

func TestURLRouting(t *testing.T) {
	client := downloader.NewClient()

	tests := []struct {
		url     string
		wantErr bool
	}{
		{"", true},
		{"invalid-url-string", true},
		{"https://example.com/test", true},
		{"https://unknown-service.org/item/123", true},
	}

	for _, tt := range tests {
		_, err := client.Download(context.Background(), tt.url)
		if (err != nil) != tt.wantErr {
			t.Errorf("Download(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
		}
	}
}

func TestFacebookExtractor(t *testing.T) {
	client := downloader.NewClient()

	testURLs := []string{
		"https://web.facebook.com/reel/730293269054758",
		"https://web.facebook.com/watch/?v=883839773514682&ref=sharing",
		"https://web.facebook.com/share/r/JFZfPVgLkiJQmWrr/",
	}

	for _, u := range testURLs {
		t.Run(u, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			res, err := client.DownloadFacebook(ctx, u)
			if err != nil {
				t.Logf("Facebook download for %s returned error (may be network/geo restricted): %v", u, err)
				return
			}

			if res.Service != "facebook" {
				t.Errorf("expected service 'facebook', got %s", res.Service)
			}
			if len(res.Items) == 0 {
				t.Errorf("expected at least 1 media item for %s", u)
			}
		})
	}
}

func TestInstagramExtractor(t *testing.T) {
	client := downloader.NewClient()

	testURLs := []string{
		"https://www.instagram.com/p/DFx6KVduFWy/",
		"https://www.instagram.com/reel/DFQe23tOWKz/",
		"https://www.instagram.com/p/CvYrSgnsKjv/",
	}

	for _, u := range testURLs {
		t.Run(u, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			res, err := client.DownloadInstagram(ctx, u)
			if err != nil {
				t.Logf("Instagram download for %s returned error (may be rate-limited/private): %v", u, err)
				return
			}

			if res.Service != "instagram" {
				t.Errorf("expected service 'instagram', got %s", res.Service)
			}
			if len(res.Items) == 0 {
				t.Errorf("expected at least 1 media item for %s", u)
			}
		})
	}
}

func TestTwitterExtractor(t *testing.T) {
	client := downloader.NewClient()

	testURLs := []struct {
		url          string
		expectedType string
	}{
		{
			url:          "https://twitter.com/X/status/1697304622749086011",
			expectedType: "video",
		},
		{
			url:          "https://x.com/PopCrave/status/1815960083475423235",
			expectedType: "photo",
		},
		{
			url:          "https://x.com/PopCrave/status/1877880433242771717",
			expectedType: "photo",
		},
	}

	for _, tt := range testURLs {
		t.Run(tt.url, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			res, err := client.DownloadTwitter(ctx, tt.url)
			if err != nil {
				t.Logf("Twitter download for %s returned error: %v", tt.url, err)
				return
			}

			if res.Service != "twitter" {
				t.Errorf("expected service 'twitter', got %s", res.Service)
			}
			if len(res.Items) == 0 {
				t.Errorf("expected at least 1 media item for %s", tt.url)
			}
			if !strings.HasPrefix(res.Items[0].URL, "http") {
				t.Errorf("expected valid HTTP URL, got %s", res.Items[0].URL)
			}
		})
	}
}

func TestTwitterSSSTwitterFallback(t *testing.T) {
	client := downloader.NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	res, err := client.DownloadTwitter(ctx, "https://x.com/i/status/2084652239524696166")
	if err != nil {
		t.Fatalf("SSSTwitter fallback returned error: %v", err)
	}
	if res.Service != "twitter" {
		t.Errorf("expected service 'twitter', got %s", res.Service)
	}
	if len(res.Items) == 0 {
		t.Errorf("expected at least 1 media item")
	}
}

func TestTikTokExtractor(t *testing.T) {
	client := downloader.NewClient()

	testURLs := []string{
		"https://www.tiktok.com/@fatfatmillycat/video/7195741644585454894",
		"https://www.tiktok.com/@matryoshk4/video/7231234675476532526",
	}

	for _, u := range testURLs {
		t.Run(u, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			res, err := client.DownloadTikTok(ctx, u)
			if err != nil {
				t.Logf("TikTok download for %s returned error: %v", u, err)
				return
			}

			if res.Service != "tiktok" {
				t.Errorf("expected service 'tiktok', got %s", res.Service)
			}
			if len(res.Items) == 0 {
				t.Errorf("expected at least 1 media item for %s", u)
			}
		})
	}
}

func TestSnapchatExtractor(t *testing.T) {
	client := downloader.NewClient()

	testURLs := []string{
		"https://www.snapchat.com/spotlight/W7_EDlXWTBiXAEEniNoMPwAAYdWxucG9pZmNqAY46j0a5AY46j0YbAAAAAQ",
		"https://www.snapchat.com/add/bazerkmakane",
	}

	for _, u := range testURLs {
		t.Run(u, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			res, err := client.DownloadSnapchat(ctx, u)
			if err != nil {
				t.Logf("Snapchat download for %s returned error: %v", u, err)
				return
			}

			if res.Service != "snapchat" {
				t.Errorf("expected service 'snapchat', got %s", res.Service)
			}
			if len(res.Items) == 0 {
				t.Errorf("expected at least 1 media item for %s", u)
			}
		})
	}
}

func TestRedditExtractor(t *testing.T) {
	client := downloader.NewClient()

	testURLs := []string{
		"https://www.reddit.com/r/TikTokCringe/comments/wup1fg/id_be_escaping_at_the_first_chance_i_got/",
		"https://www.reddit.com/r/whenthe/comments/109wqy1/god_really_did_some_trolling/",
	}

	for _, u := range testURLs {
		t.Run(u, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			res, err := client.DownloadReddit(ctx, u)
			if err != nil {
				t.Logf("Reddit download for %s returned error: %v", u, err)
				return
			}

			if res.Service != "reddit" {
				t.Errorf("expected service 'reddit', got %s", res.Service)
			}
			if len(res.Items) == 0 {
				t.Errorf("expected at least 1 media item for %s", u)
			}
		})
	}
}

func TestPinterestExtractor(t *testing.T) {
	client := downloader.NewClient()

	testURLs := []string{
		"https://www.pinterest.com/pin/70437485604616/",
		"https://www.pinterest.com/pin/412994228343400946/",
		"https://www.pinterest.com/pin/643170390530326178/",
	}

	for _, u := range testURLs {
		t.Run(u, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			res, err := client.DownloadPinterest(ctx, u)
			if err != nil {
				t.Logf("Pinterest download for %s returned error: %v", u, err)
				return
			}

			if res.Service != "pinterest" {
				t.Errorf("expected service 'pinterest', got %s", res.Service)
			}
			if len(res.Items) == 0 {
				t.Errorf("expected at least 1 media item for %s", u)
			}
		})
	}
}

func TestSoundCloudExtractor(t *testing.T) {
	client := downloader.NewClient()

	testURLs := []string{
		"https://soundcloud.com/l2share77/loona-butterfly",
		"https://on.soundcloud.com/XHLLKSXRQ5yyGDuD9",
	}

	for _, u := range testURLs {
		t.Run(u, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			res, err := client.DownloadSoundCloud(ctx, u)
			if err != nil {
				t.Logf("SoundCloud download for %s returned error: %v", u, err)
				return
			}

			if res.Service != "soundcloud" {
				t.Errorf("expected service 'soundcloud', got %s", res.Service)
			}
			if len(res.Items) == 0 {
				t.Errorf("expected at least 1 media item for %s", u)
			}
		})
	}
}

func TestThreadsExtractor(t *testing.T) {
	client := downloader.NewClient()

	testURLs := []string{
		"https://www.threads.net/@zuck/post/C9WfX36S5l5",
	}

	for _, u := range testURLs {
		t.Run(u, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			res, err := client.DownloadThreads(ctx, u)
			if err != nil {
				t.Logf("Threads download for %s returned error: %v", u, err)
				return
			}

			if res.Service != "threads" {
				t.Errorf("expected service 'threads', got %s", res.Service)
			}
			if len(res.Items) == 0 {
				t.Errorf("expected at least 1 media item for %s", u)
			}
		})
	}
}

func TestBlueskyExtractor(t *testing.T) {
	client := downloader.NewClient()

	testURLs := []string{
		"https://bsky.app/profile/bsky.app/post/3kgklyb7kss2x",
	}

	for _, u := range testURLs {
		t.Run(u, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			res, err := client.DownloadBluesky(ctx, u)
			if err != nil {
				t.Logf("Bluesky download for %s returned error: %v", u, err)
				return
			}

			if res.Service != "bluesky" {
				t.Errorf("expected service 'bluesky', got %s", res.Service)
			}
			if len(res.Items) == 0 {
				t.Errorf("expected at least 1 media item for %s", u)
			}
		})
	}
}

func TestVKExtractor(t *testing.T) {
	client := downloader.NewClient()

	testURLs := []string{
		"https://vk.com/video-22822305_456239162",
	}

	for _, u := range testURLs {
		t.Run(u, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			res, err := client.DownloadVK(ctx, u)
			if err != nil {
				t.Logf("VK download for %s returned error: %v", u, err)
				return
			}

			if res.Service != "vk" {
				t.Errorf("expected service 'vk', got %s", res.Service)
			}
			if len(res.Items) == 0 {
				t.Errorf("expected at least 1 media item for %s", u)
			}
		})
	}
}

func TestTumblrExtractor(t *testing.T) {
	client := downloader.NewClient()

	testURLs := []string{
		"https://www.tumblr.com/staff/72123456789/test",
	}

	for _, u := range testURLs {
		t.Run(u, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			res, err := client.DownloadTumblr(ctx, u)
			if err != nil {
				t.Logf("Tumblr download for %s returned error: %v", u, err)
				return
			}

			if res.Service != "tumblr" {
				t.Errorf("expected service 'tumblr', got %s", res.Service)
			}
		})
	}
}

func TestTwitchExtractor(t *testing.T) {
	client := downloader.NewClient()

	testURLs := []string{
		"https://clips.twitch.tv/GloriousSillyWaffleDogFace-x7m-F8Gk9y1m9m1m",
	}

	for _, u := range testURLs {
		t.Run(u, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			res, err := client.DownloadTwitch(ctx, u)
			if err != nil {
				t.Logf("Twitch download for %s returned error: %v", u, err)
				return
			}

			if res.Service != "twitch" {
				t.Errorf("expected service 'twitch', got %s", res.Service)
			}
		})
	}
}

func TestYouTubeMediaExtractor(t *testing.T) {
	client := downloader.NewClient()
	testURL := "https://www.youtube.com/watch?v=rScwLoES2bM"

	t.Run("AudioOnly", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		res, err := client.DownloadYouTubeMedia(ctx, testURL, true)
		if err != nil {
			t.Fatalf("DownloadYouTubeMedia audio failed: %v", err)
		}
		if res.Service != "youtube" {
			t.Errorf("expected service 'youtube', got %s", res.Service)
		}
		if len(res.Items) == 0 || res.Items[0].URL == "" {
			t.Errorf("expected valid stream URL for audio")
		}
	})

	t.Run("Video", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		res, err := client.DownloadYouTubeMedia(ctx, testURL, false)
		if err != nil {
			t.Fatalf("DownloadYouTubeMedia video failed: %v", err)
		}
		if res.Service != "youtube" {
			t.Errorf("expected service 'youtube', got %s", res.Service)
		}
		if len(res.Items) == 0 || res.Items[0].URL == "" {
			t.Errorf("expected valid stream URL for video")
		}
	})
}
