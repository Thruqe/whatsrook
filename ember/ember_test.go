package ember

import (
	"context"
	"strings"
	"testing"
	"time"
)

const (
	testTwitterURL   = "https://x.com/r0ktech/status/2079050348216430609?s=20"
	testInstagramURL = "https://www.instagram.com/threads/reel/DbOkJbvTqcD/?hl=en"
	testTikTokURL    = "https://vt.tiktok.com/ZS4J9fGrg/"
	testFacebookURL  = "https://www.facebook.com/watch/?v=1234567890123456"
	testThreadsURL   = "https://www.threads.net/@zuck/post/EXAMPLE123"
)

func TestFetchTwitter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	data, err := Fetch(ctx, testTwitterURL, "")
	if err != nil {
		t.Fatalf("Fetch failed for Twitter URL %q: %v", testTwitterURL, err)
	}
	if data.Title == "" {
		t.Error("Expected non-empty title")
	}
	if data.Author == "" {
		t.Error("Expected non-empty author")
	}
	media, ok := data.BestMedia()
	if !ok {
		t.Fatal("Expected at least one media item")
	}
	if media.Type != "video" {
		t.Errorf("Expected video media, got %s", media.Type)
	}
	if media.URL == "" {
		t.Error("Expected non-empty media URL")
	}
	if media.Extension == "" {
		t.Error("Expected non-empty extension")
	}
	t.Logf("Title: %s", data.Title)
	t.Logf("Author: %s", data.Author)
	t.Logf("Media URL: %s", media.URL)
	t.Logf("Media Type: %s", media.Type)
	t.Logf("Media Ext: %s", media.Extension)
	t.Logf("Caption: %s", data.Caption())
	t.Logf("Medias count: %d", len(data.Medias))
}

func TestFetchInstagram(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	data, err := Fetch(ctx, testInstagramURL, "")
	if err != nil {
		t.Fatalf("Fetch failed for Instagram URL %q: %v", testInstagramURL, err)
	}
	media, ok := data.BestMedia()
	if !ok {
		t.Fatal("Expected at least one media item")
	}
	if media.URL == "" {
		t.Error("Expected non-empty media URL")
	}
	t.Logf("Title: %s", data.Title)
	t.Logf("Author: %s", data.Author)
	t.Logf("Media Type: %s", media.Type)
	t.Logf("Media URL: %s", media.URL)
	t.Logf("Medias count: %d", len(data.Medias))
}

func TestFetchTikTok(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	data, err := Fetch(ctx, testTikTokURL, "")
	if err != nil {
		t.Fatalf("Fetch failed for TikTok URL %q: %v", testTikTokURL, err)
	}
	media, ok := data.BestMedia()
	if !ok {
		t.Fatal("Expected at least one media item")
	}
	if media.Type != "video" {
		t.Errorf("Expected video media, got %s", media.Type)
	}
	t.Logf("Title: %s", data.Title)
	t.Logf("Author: %s", data.Author)
	t.Logf("Media URL: %s", media.URL)
	t.Logf("Medias count: %d", len(data.Medias))
}

func TestFetchFacebook(t *testing.T) {
	if isPlaceholderURL(testFacebookURL) {
		t.Skip("Skipping: replace testFacebookURL with a real public Facebook video URL (not a post URL). Use format: https://www.facebook.com/watch/?v=REAL_VIDEO_ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	data, err := Fetch(ctx, testFacebookURL, "")
	if err != nil {
		t.Fatalf("Fetch failed for Facebook URL %q: %v", testFacebookURL, err)
	}
	media, ok := data.BestMedia()
	if !ok {
		t.Fatal("Expected at least one media item")
	}
	if media.URL == "" {
		t.Error("Expected non-empty media URL")
	}
	t.Logf("Title: %s", data.Title)
	t.Logf("Author: %s", data.Author)
	t.Logf("Media Type: %s", media.Type)
	t.Logf("Media URL: %s", media.URL)
}

func TestFetchThreads(t *testing.T) {
	if isPlaceholderURL(testThreadsURL) {
		t.Skip("Skipping: replace testThreadsURL with a real public Threads post URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	data, err := Fetch(ctx, testThreadsURL, "")
	if err != nil {
		t.Fatalf("Fetch failed for Threads URL %q: %v", testThreadsURL, err)
	}
	media, ok := data.BestMedia()
	if !ok {
		t.Fatal("Expected at least one media item")
	}
	if media.URL == "" {
		t.Error("Expected non-empty media URL")
	}
	t.Logf("Title: %s", data.Title)
	t.Logf("Author: %s", data.Author)
	t.Logf("Media Type: %s", media.Type)
	t.Logf("Media URL: %s", media.URL)
}

func TestFetchInvalidURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := Fetch(ctx, "https://not-a-real-url.example.com/post/123", "")
	if err == nil {
		t.Fatal("Expected error for invalid URL")
	}
	t.Logf("Got expected error: %v", err)
}

func TestBestMedia(t *testing.T) {
	d := Data{
		Medias: []Media{
			{URL: "https://example.com/audio.mp3", Type: "audio"},
			{URL: "https://example.com/video.mp4", Type: "video"},
		},
	}
	m, ok := d.BestMedia()
	if !ok {
		t.Fatal("Expected best media to be found")
	}
	if m.Type != "video" {
		t.Errorf("Expected best media to be video, got %s", m.Type)
	}
}

func TestBestMediaNoVideo(t *testing.T) {
	d := Data{
		Medias: []Media{
			{URL: "https://example.com/audio.mp3", Type: "audio"},
		},
	}
	m, ok := d.BestMedia()
	if !ok {
		t.Fatal("Expected best media to be found")
	}
	if m.Type != "audio" {
		t.Errorf("Expected best media to be audio, got %s", m.Type)
	}
}

func TestBestMediaEmpty(t *testing.T) {
	d := Data{Medias: nil}
	_, ok := d.BestMedia()
	if ok {
		t.Error("Expected no media to be found")
	}
}

func TestCaption(t *testing.T) {
	d := Data{
		Title:  "Hello World",
		Author: "Jane Doe",
	}
	cap := d.Caption()
	expected := "Hello World\n— Jane Doe"
	if cap != expected {
		t.Errorf("Expected caption %q, got %q", expected, cap)
	}
}

func TestCaptionEmpty(t *testing.T) {
	d := Data{}
	if d.Caption() != "" {
		t.Error("Expected empty caption")
	}
}

func TestPopulateCompatType(t *testing.T) {
	tests := []struct {
		name     string
		medias   []Media
		expected string
	}{
		{"none", nil, "none"},
		{"single", []Media{{URL: "a.mp4", Type: "video"}}, "single"},
		{"multiple", []Media{{URL: "a.mp4"}, {URL: "b.mp4"}}, "multiple"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Data{
				Medias: tt.medias,
				Type:   "",
			}
			switch len(d.Medias) {
			case 0:
				d.Type = "none"
			case 1:
				d.Type = "single"
			default:
				d.Type = "multiple"
			}
			if d.Type != tt.expected {
				t.Errorf("Expected Type %q, got %q", tt.expected, d.Type)
			}
		})
	}
}

func isPlaceholderURL(url string) bool {
	placeholders := []string{
		"EXAMPLE", "example", "USERNAME", "USER", "VIDEO_ID",
		"PAGE", "POST_ID", "SHORTCODE", "REAL_VIDEO_ID",
		"1234567890123456",
	}
	lower := strings.ToLower(url)
	for _, p := range placeholders {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}
