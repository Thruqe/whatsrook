// Shared utility functions: FFmpeg transcoding, ffprobe duration, URL matching,
// JID sanitisation, and message extraction.
package utils

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

// TranscodeToMP3 converts any input audio file to MP3 via ffmpeg, returning the
// new file's path. WhatsApp voice notes come as Ogg/Opus, which meowcaller's
// OpusFile can't reliably play back (silent output) — MP3 works cleanly instead.
func TranscodeToMP3(inputPath string) (string, error) {
	outputPath := strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ".mp3"
	actualOut := outputPath
	if outputPath == inputPath {
		actualOut = inputPath + ".tmp.mp3"
	}

	cmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-ar", "16000", "-ac", "1", actualOut)
	if out, err := cmd.CombinedOutput(); err != nil {
		if outputPath == inputPath {
			_ = os.Remove(actualOut)
		}
		return "", fmt.Errorf("ffmpeg transcode failed: %w (%s)", err, string(out))
	}

	if outputPath == inputPath {
		if err := os.Rename(actualOut, inputPath); err != nil {
			return "", fmt.Errorf("rename transcoded file: %w", err)
		}
	}

	return outputPath, nil
}

// IsSaveText checks if a text string matches our save trigger word.
func IsSaveText(text string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(text)), "save")
}

// GetDirectMessageText safely pulls text strings out of a top-level native message.
func GetDirectMessageText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	var sb strings.Builder
	if msg.GetExtendedTextMessage() != nil {
		sb.WriteString(" ")
		sb.WriteString(msg.GetExtendedTextMessage().GetText())
	}
	if msg.GetConversation() != "" {
		sb.WriteString(" ")
		sb.WriteString(msg.GetConversation())
	}
	return sb.String()
}

// ExtensionFor returns file extension based on mimetype.
func ExtensionFor(mimetype string) string {
	var ext string
	switch {
	case strings.Contains(mimetype, "ogg"):
		ext = ".ogg"
	case strings.Contains(mimetype, "mpeg"), strings.Contains(mimetype, "mp3"):
		ext = ".mp3"
	case strings.Contains(mimetype, "wav"):
		ext = ".wav"
	default:
		ext = ".bin"
	}
	log.Printf("[DEBUG] Mapped mimetype %q to extension %q", mimetype, ext)
	return ext
}

// SanitizeJID replaces characters in JID to make it safe for file paths.
func SanitizeJID(s string) string {
	res := strings.NewReplacer("@", "_at_", ":", "_", ".", "_").Replace(s)
	log.Printf("[DEBUG] Sanitized JID from %s to %s", s, res)
	return res
}

// AudioDuration uses ffprobe to get an audio file's duration.
func AudioDuration(path string) (time.Duration, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// IsFacebookURL checks if the URL matches Facebook domain.
func IsFacebookURL(link string) bool {
	return MatchesHost(link, "facebook.com", "fb.com", "fb.watch")
}

// IsInstagramURL checks if the URL matches Instagram domain.
func IsInstagramURL(link string) bool {
	return MatchesHost(link, "instagram.com")
}

// IsTwitterURL checks if the URL matches Twitter/X domain.
func IsTwitterURL(link string) bool {
	return MatchesHost(link, "twitter.com", "x.com")
}

// IsThreadsURL checks if the URL matches Threads domain.
func IsThreadsURL(link string) bool {
	return MatchesHost(link, "threads.net", "threads.com")
}

// IsYouTubeURL checks if the URL matches YouTube domain.
func IsYouTubeURL(link string) bool {
	return MatchesHost(link, "youtube.com", "youtu.be")
}

// IsTikTokURL checks if the URL matches TikTok domain.
func IsTikTokURL(link string) bool {
	return MatchesHost(link, "tiktok.com")
}

// MatchesHost parses the URL and checks if its host matches
// any of the given domains (including subdomains like www.).
func MatchesHost(link string, domains ...string) bool {
	u, err := url.Parse(strings.TrimSpace(link))
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimPrefix(host, "m.")

	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// GetGitCommit returns the short commit hash if running inside a Git repository.
func GetGitCommit() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "N/A"
	}
	return strings.TrimSpace(string(out))
}

// SystemMetadata contains runtime system and environment details.
type SystemMetadata struct {
	Version   string
	Commit    string
	OS        string
	Arch      string
	NumCPU    int
	GoVersion string
}

// GetSystemMetadata gathers system metadata for diagnostics and status reporting.
func GetSystemMetadata(version string) SystemMetadata {
	commit := "N/A"
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	if out, err := cmd.Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	}

	return SystemMetadata{
		Version:   version,
		Commit:    commit,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		NumCPU:    runtime.NumCPU(),
		GoVersion: runtime.Version(),
	}
}
