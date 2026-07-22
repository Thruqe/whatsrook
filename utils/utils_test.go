package utils_test

import (
	"testing"

	"whatsrook/utils"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

func TestURLMatching(t *testing.T) {
	tests := []struct {
		url      string
		fn       func(string) bool
		expected bool
	}{
		{"https://facebook.com/watch?v=123", utils.IsFacebookURL, true},
		{"https://fb.watch/123", utils.IsFacebookURL, true},
		{"https://instagram.com/reel/123", utils.IsInstagramURL, true},
		{"https://twitter.com/user/status/123", utils.IsTwitterURL, true},
		{"https://x.com/user/status/123", utils.IsTwitterURL, true},
		{"https://threads.net/@user/post/123", utils.IsThreadsURL, true},
		{"https://youtube.com/watch?v=123", utils.IsYouTubeURL, true},
		{"https://youtu.be/123", utils.IsYouTubeURL, true},
		{"https://tiktok.com/@user/video/123", utils.IsTikTokURL, true},
		{"https://example.com", utils.IsFacebookURL, false},
	}

	for _, tt := range tests {
		if res := tt.fn(tt.url); res != tt.expected {
			t.Errorf("URL %s matching failed: expected %v, got %v", tt.url, tt.expected, res)
		}
	}
}

func TestIsSaveText(t *testing.T) {
	if !utils.IsSaveText("  SAVE  ") {
		t.Errorf("expected true for '  SAVE  '")
	}
	if !utils.IsSaveText("please save this audio") {
		t.Errorf("expected true for 'please save this audio'")
	}
	if utils.IsSaveText("hello world") {
		t.Errorf("expected false for 'hello world'")
	}
}

func TestSanitizeJID(t *testing.T) {
	input := "123456:7@s.whatsapp.net"
	expected := "123456_7_at_s_whatsapp_net"
	if res := utils.SanitizeJID(input); res != expected {
		t.Errorf("SanitizeJID(%q) = %q; expected %q", input, res, expected)
	}
}

func TestExtensionFor(t *testing.T) {
	if ext := utils.ExtensionFor("audio/ogg"); ext != ".ogg" {
		t.Errorf("expected .ogg, got %s", ext)
	}
	if ext := utils.ExtensionFor("audio/mp3"); ext != ".mp3" {
		t.Errorf("expected .mp3, got %s", ext)
	}
	if ext := utils.ExtensionFor("audio/wav"); ext != ".wav" {
		t.Errorf("expected .wav, got %s", ext)
	}
	if ext := utils.ExtensionFor("application/octet-stream"); ext != ".bin" {
		t.Errorf("expected .bin, got %s", ext)
	}
}

func TestGetDirectMessageText(t *testing.T) {
	conv := "hello"
	msg := &waE2E.Message{
		Conversation: &conv,
	}
	if text := utils.GetDirectMessageText(msg); text != " hello" {
		t.Errorf("expected ' hello', got %q", text)
	}
}
