package utils_test

import (
	"context"
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

func TestSplitAnnexBAccessUnits(t *testing.T) {
	// Keyframe AU: AUD(9), SPS(7), PPS(8), IDR(5)
	// Delta AU: AUD(9), Slice(1)
	stream := []byte{
		0, 0, 0, 1, 9, 0x10, // AUD
		0, 0, 0, 1, 7, 0x42, 0x00, // SPS
		0, 0, 0, 1, 8, 0xce, // PPS
		0, 0, 0, 1, 5, 0x01, 0x02, // IDR
		0, 0, 0, 1, 9, 0x10, // AUD
		0, 0, 0, 1, 1, 0x03, 0x04, // Slice
	}

	units := utils.SplitAnnexBAccessUnits(stream)
	if len(units) != 2 {
		t.Fatalf("got %d units, want 2", len(units))
	}
	if !utils.AnnexBHasIDR(units[0]) {
		t.Errorf("unit 0 should have IDR keyframe")
	}
	if utils.AnnexBHasIDR(units[1]) {
		t.Errorf("unit 1 should be delta frame")
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

func TestEnsureOpusPTT_Empty(t *testing.T) {
	ctx := context.Background()
	meta, err := utils.EnsureOpusPTT(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error on nil input: %v", err)
	}
	if meta == nil || len(meta.Data) != 0 {
		t.Errorf("expected empty output for nil input, got %+v", meta)
	}
}

func TestWaveformExtraction(t *testing.T) {
	// Generate 1 second of 8kHz 16-bit PCM (8000 samples = 16000 bytes)
	pcm := make([]byte, 16000)
	for i := 0; i < 8000; i++ {
		// First 4000 samples low volume, last 4000 samples high volume
		val := int16(1000)
		if i >= 4000 {
			val = int16(30000)
		}
		pcm[i*2] = byte(uint16(val))
		pcm[i*2+1] = byte(uint16(val) >> 8)
	}

	sec, waveform := utils.ExtractWaveformForTest(pcm, 8000)
	if sec != 1 {
		t.Errorf("expected 1 second, got %d", sec)
	}
	if len(waveform) != 64 {
		t.Fatalf("expected 64 waveform bins, got %d", len(waveform))
	}
	// Second half should be scaled to peak 100
	if waveform[63] != 100 {
		t.Errorf("expected peak bin to be 100, got %d", waveform[63])
	}
	// First half average should be lower than second half
	if waveform[10] >= waveform[50] {
		t.Errorf("expected first half average (%d) to be strictly lower than second half (%d)", waveform[10], waveform[50])
	}
}
