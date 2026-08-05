package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tcolgate/mp3"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

// TranscodeToMP3 converts input audio to MP3 via ffmpeg CLI.
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

// PrepareCallVideo converts video to audio (.mp3) and Annex-B H.264 video stream (.h264) via ffmpeg CLI.
func PrepareCallVideo(inputPath string) (string, string, error) {
	basePath := strings.TrimSuffix(inputPath, filepath.Ext(inputPath))
	mp3Path := basePath + ".mp3"
	h264Path := basePath + ".h264"

	// 1. Audio Extraction
	audioCmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-vn", "-ar", "16000", "-ac", "1", "-b:a", "64k", mp3Path)
	if out, err := audioCmd.CombinedOutput(); err != nil {
		log.Printf("[WARN] ffmpeg audio extraction failed for %s: %v (%s)", inputPath, err, string(out))
	}

	// 2. Video Transcode to Annex-B H.264 via ffmpeg CLI
	videoCmd := exec.Command("ffmpeg", "-y", "-i", inputPath, "-an", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-r", "15", "-g", "15", "-bsf:v", "h264_mp4toannexb", "-f", "h264", h264Path)
	if out, err := videoCmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("ffmpeg video transcode failed for %s: %w (%s)", inputPath, err, string(out))
	}

	return mp3Path, h264Path, nil
}

// AudioDuration calculates MP3 duration in pure Go by reading frame headers.
func AudioDuration(path string) (time.Duration, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open audio file: %w", err)
	}
	defer file.Close()

	var totalDuration float64
	decoder := mp3.NewDecoder(file)
	var frame mp3.Frame
	var skipped int

	for {
		if err := decoder.Decode(&frame, &skipped); err != nil {
			if err == io.EOF {
				break
			}
			return 0, fmt.Errorf("decode mp3 frame: %w", err)
		}
		totalDuration += frame.Duration().Seconds()
	}

	return time.Duration(totalDuration * float64(time.Second)), nil
}

func annexBStartCodeLen(data []byte, i int) int {
	if i+3 < len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
		return 4
	}
	if i+2 < len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 1 {
		return 3
	}
	return 0
}

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

func IsSaveText(text string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(text)), "save")
}

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

func SanitizeJID(s string) string {
	res := strings.NewReplacer("@", "_at_", ":", "_", ".", "_").Replace(s)
	log.Printf("[DEBUG] Sanitized JID from %s to %s", s, res)
	return res
}
