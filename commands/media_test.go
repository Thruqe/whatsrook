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
		{"", "WhatsRook Pack", "WhatsRook Bot"},
		{"My Pack", "My Pack", "WhatsRook Bot"},
		{"My Pack|My Author", "My Pack", "My Author"},
		{"   My Pack   |   My Author   ", "My Pack", "My Author"},
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
	exif := generateStickerExif(pack, auth)

	// Check headers
	if !bytes.HasPrefix(exif, []byte("Exif\x00\x00II\x2a\x00")) {
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
