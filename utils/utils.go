package utils

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// EnsureJPEG decodes input image bytes (JPEG, PNG, GIF, WebP, etc.) and re-encodes to valid JPEG bytes.
func EnsureJPEG(ctx context.Context, inputBytes []byte) ([]byte, error) {
	if len(inputBytes) == 0 {
		return inputBytes, nil
	}

	img, _, err := image.Decode(bytes.NewReader(inputBytes))
	if err == nil {
		var buf bytes.Buffer
		if errEnc := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); errEnc == nil && buf.Len() > 0 {
			return buf.Bytes(), nil
		}
	}

	// Fallback via ffmpeg for WebP / HEIC / MP4 / etc.
	tmpIn := filepath.Join(os.TempDir(), fmt.Sprintf("pfp_in_%d.bin", time.Now().UnixNano()))
	tmpOut := filepath.Join(os.TempDir(), fmt.Sprintf("pfp_out_%d.jpg", time.Now().UnixNano()))

	if err := os.WriteFile(tmpIn, inputBytes, 0644); err != nil {
		return inputBytes, nil
	}
	defer os.Remove(tmpIn)
	defer os.Remove(tmpOut)

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", tmpIn, "-vframes", "1", "-q:v", "2", tmpOut)
	if err := cmd.Run(); err == nil {
		if converted, errRead := os.ReadFile(tmpOut); errRead == nil && len(converted) > 0 {
			return converted, nil
		}
	}

	return inputBytes, nil
}

// FetchURLBytes fetches the content of a target URL over HTTP GET.
func FetchURLBytes(ctx context.Context, targetURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status %d fetching URL", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// FormatBytes converts a byte count into a human-readable string (KB, MB, GB).
func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// FormatUptime converts seconds into a human-readable duration string (e.g. 1d 2h 3m 4s).
func FormatUptime(seconds float64) string {
	totalSec := int(seconds)
	days := totalSec / (24 * 3600)
	totalSec %= (24 * 3600)
	hours := totalSec / 3600
	totalSec %= 3600
	mins := totalSec / 60
	secs := totalSec % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if mins > 0 {
		parts = append(parts, fmt.Sprintf("%dm", mins))
	}
	if secs > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", secs))
	}
	return strings.Join(parts, " ")
}

// IsKnownLanguageCode checks if a language code or name is valid for TTS.
func IsKnownLanguageCode(lang string) bool {
	clean := strings.ToLower(strings.TrimSpace(lang))
	if clean == "" {
		return false
	}
	// Common ISO language codes
	known := map[string]bool{
		"en": true, "es": true, "fr": true, "de": true, "it": true, "pt": true,
		"ru": true, "ja": true, "ko": true, "zh": true, "ar": true, "hi": true,
		"tr": true, "nl": true, "pl": true, "sv": true, "id": true, "th": true,
		"vi": true, "he": true, "uk": true, "cs": true, "el": true, "hu": true,
		"ro": true, "sk": true, "da": true, "fi": true, "no": true, "sw": true,
	}
	return known[clean] || len(clean) == 2 || len(clean) == 5
}
