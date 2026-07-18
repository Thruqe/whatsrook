package commands

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func sendText(ctx *Context, text string) error {
	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, &waE2E.Message{
		Conversation: new(text),
	})
	return err
}

// sendTextRaw is like sendText but usable before a *Context exists (e.g. inside
// HandlePendingAudioReply, which runs ahead of normal command dispatch).
func sendTextRaw(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string) error {
	_, err := client.SendMessage(ctx, chat, &waE2E.Message{
		Conversation: new(text),
	})
	return err
}

// transcodeToMP3 converts any input audio file to MP3 via ffmpeg, returning the
// new file's path. WhatsApp voice notes come as Ogg/Opus, which meowcaller's
// OpusFile can't reliably play back (silent output) — MP3 works cleanly instead.
func transcodeToMP3(inputPath string) (string, error) {
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

// Helper to check if a text string matches our save trigger word
func isSaveText(text string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(text)), "save")
}

// Helper to safely pull text strings out of a top-level native message
func getDirectMessageText(msg *waE2E.Message) string {
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

func extensionFor(mimetype string) string {
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

func sanitizeJID(s string) string {
	res := strings.NewReplacer("@", "_at_", ":", "_", ".", "_").Replace(s)
	log.Printf("[DEBUG] Sanitized JID from %s to %s", s, res)
	return res
}

// audioDuration uses ffprobe to get an audio file's duration.
func audioDuration(path string) (time.Duration, error) {
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

func isFacebookURL(link string) bool {
	return matchesHost(link, "facebook.com", "fb.com", "fb.watch")
}

func isInstagramURL(link string) bool {
	return matchesHost(link, "instagram.com")
}

func isThreadsURL(link string) bool {
	return matchesHost(link, "threads.net")
}

func isYouTubeURL(link string) bool {
	return matchesHost(link, "youtube.com", "youtu.be")
}

func isTikTokURL(link string) bool {
	return matchesHost(link, "tiktok.com")
}

// matchesHost parses the URL and checks if its host matches
// any of the given domains (including subdomains like www.).
func matchesHost(link string, domains ...string) bool {
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
