package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseStickerMetadata(t *testing.T) {
	tests := []struct {
		raw      string
		wantPack string
		wantAuth string
	}{
		{"", "WhatsRook", "Thruqe"},
		{"My Author", "WhatsRook", "My Author"},
		{"My Author|My Pack", "My Pack", "My Author"},
		{"   My Author   |   My Pack   ", "My Pack", "My Author"},
	}

	for _, tt := range tests {
		gotPack, gotAuth := parseStickerMetadata(tt.raw)
		if gotPack != tt.wantPack || gotAuth != tt.wantAuth {
			t.Errorf("parseStickerMetadata(%q) = (%q, %q); want (%q, %q)", tt.raw, gotPack, gotAuth, tt.wantPack, tt.wantAuth)
		}
	}
}

func TestGenerateStickerExif(t *testing.T) {
	pack := "Test Pack"
	auth := "Test Author"
	meta := &exifStickerMetadata{
		PackName:  pack,
		Publisher: auth,
	}
	exif, err := buildExif(meta)
	if err != nil {
		t.Fatalf("failed to build exif: %v", err)
	}

	// Check headers: must start with Little-endian TIFF ("II\x2a\x00")
	if !bytes.HasPrefix(exif, []byte("II\x2a\x00")) {
		t.Errorf("exif has invalid header: %q", exif[:12])
	}

	// Check if json is embedded
	exifStr := string(exif)
	if !strings.Contains(exifStr, pack) {
		t.Errorf("exif metadata does not contain pack name: %s", exifStr)
	}
	if !strings.Contains(exifStr, auth) {
		t.Errorf("exif metadata does not contain author name: %s", exifStr)
	}
}
