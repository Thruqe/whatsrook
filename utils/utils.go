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

// PrepareCallVideo converts any input video file to both a WhatsApp-compatible
// audio track (.mp3) and an Annex-B H.264 video stream (.h264) via ffmpeg.
func PrepareCallVideo(inputPath string) (string, string, error) {
	basePath := strings.TrimSuffix(inputPath, filepath.Ext(inputPath))
	mp3Path := basePath + ".mp3"
	h264Path := basePath + ".h264"

	// 1. Extract/Transcode Audio to MP3 (16kHz mono 64k)
	audioCmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-vn", "-ar", "16000", "-ac", "1", "-b:a", "64k", mp3Path)
	if out, err := audioCmd.CombinedOutput(); err != nil {
		log.Printf("[WARN] ffmpeg audio extraction failed for %s: %v (%s)", inputPath, err, string(out))
	}

	// 2. Transcode Video to Annex-B H.264 (yuv420p baseline 15 FPS).
	// - keyint=15:min-keyint=15 forces IDR every 15 frames.
	// - repeat-headers=1 prepends SPS+PPS to every IDR NAL.
	// - aud=1 inserts an Access Unit Delimiter (NAL type 9) before every frame;
	//   this gives SplitAnnexBAccessUnits a reliable frame boundary marker.
	// - bsf h264_mp4toannexb converts from AVCC length-prefixed to start-code format.
	videoCmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-an",
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-profile:v", "baseline",
		"-level", "3.0",
		"-preset", "ultrafast",
		"-x264opts", "keyint=15:min-keyint=15:no-scenecut=1:repeat-headers=1:aud=1",
		"-bsf:v", "h264_mp4toannexb",
		"-r", "15",
		h264Path,
	)
	if out, err := videoCmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("ffmpeg video transcode failed: %w (%s)", err, string(out))
	}

	return mp3Path, h264Path, nil
}

// annexBStartCodeLen returns the length of an Annex-B start code (3 or 4), or 0.
func annexBStartCodeLen(data []byte, i int) int {
	if i+3 < len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
		return 4
	}
	if i+2 < len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
		return 3
	}
	return 0
}

// SplitAnnexBAccessUnits groups a raw H.264 Annex-B byte stream into per-frame
// access units. A new access unit boundary is detected when an AUD (NAL type 9)
// or SPS (type 7) is encountered, or when a slice NAL (type 1 or 5) begins after
// a slice NAL has already been included in the current unit. Each returned slice
// is a complete Annex-B access unit (keyframe access units include SPS+PPS+IDR).
func SplitAnnexBAccessUnits(data []byte) [][]byte {
	var units [][]byte
	auStart := -1
	hasSliceInAU := false
	i := 0
	for i < len(data) {
		sc := annexBStartCodeLen(data, i)
		if sc == 0 {
			i++
			continue
		}
		naluStart := i + sc
		if naluStart >= len(data) {
			break
		}
		naluType := data[naluStart] & 0x1f

		// An access unit boundary occurs at AUD (9), SPS (7), or when a new slice (1 or 5)
		// begins after we already saw a slice in the current access unit.
		isSlice := naluType == 1 || naluType == 5
		isAUBoundary := naluType == 9 || naluType == 7 || (isSlice && hasSliceInAU)

		if isAUBoundary && auStart >= 0 && i > auStart {
			unit := data[auStart:i]
			if hasVideoPayload(unit) {
				units = append(units, unit)
			}
			auStart = i
			hasSliceInAU = isSlice
		} else {
			if auStart < 0 {
				auStart = i
			}
			if isSlice {
				hasSliceInAU = true
			}
		}
		i = naluStart + 1
	}
	if auStart >= 0 && auStart < len(data) {
		unit := data[auStart:]
		if hasVideoPayload(unit) {
			units = append(units, unit)
		}
	}
	return units
}

// hasVideoPayload reports whether an Annex-B chunk contains at least one slice NAL
// (IDR type 5 or non-IDR type 1), indicating a real video frame rather than just
// parameter sets or delimiters.
func hasVideoPayload(data []byte) bool {
	i := 0
	for i < len(data) {
		sc := annexBStartCodeLen(data, i)
		if sc == 0 {
			i++
			continue
		}
		naluStart := i + sc
		if naluStart >= len(data) {
			break
		}
		naluType := data[naluStart] & 0x1f
		if naluType == 5 || naluType == 1 {
			return true
		}
		i = naluStart + 1
	}
	return false
}

// AnnexBHasIDR reports whether a raw Annex-B access unit contains an IDR NAL (type 5).
// Used for diagnostic logging in the video call sender.
func AnnexBHasIDR(data []byte) bool {
	i := 0
	for i < len(data) {
		sc := annexBStartCodeLen(data, i)
		if sc == 0 {
			i++
			continue
		}
		naluStart := i + sc
		if naluStart >= len(data) {
			break
		}
		if data[naluStart]&0x1f == 5 {
			return true
		}
		i = naluStart + 1
	}
	return false
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

// GetPlatformNameFromURL returns the human-readable platform name for a URL.
func GetPlatformNameFromURL(link string) string {
	switch {
	case IsYouTubeURL(link):
		return "YouTube"
	case IsTwitterURL(link):
		return "Twitter"
	case IsInstagramURL(link):
		return "Instagram"
	case IsTikTokURL(link):
		return "TikTok"
	case IsFacebookURL(link):
		return "Facebook"
	case IsThreadsURL(link):
		return "Threads"
	default:
		return "this platform"
	}
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
