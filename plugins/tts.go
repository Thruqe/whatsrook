// Text-To-Speech (TTS) command using Google Translate TTS API.
package plugins

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"whatsrook/utils"
)

func init() {
	Register(&Command{
		Name:        "tts",
		Aliases:     []string{"say", "speech", "gtts", "speak", "voicenote"},
		Description: "Convert text into spoken voice audio using Google Speech",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleTTS,
	})
}

func handleTTS(ctx *Context) error {
	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return ctx.Reply(fmt.Sprintf("Usage: %stts <text> or %stts <lang_code> <text>\n\nExamples:\n• %stts Hello world!\n• %stts es Hola, ¿cómo estás?\n• %stts fr Bonjour tout le monde", p, p, p, p, p))
	}

	lang := "en"
	textToSpeak := ctx.RawArgs

	firstWord := strings.ToLower(ctx.Args[0])
	if len(firstWord) >= 2 && len(firstWord) <= 5 && utils.IsKnownLanguageCode(firstWord) && len(ctx.Args) > 1 {
		lang = firstWord
		textToSpeak = strings.TrimSpace(ctx.RawArgs[len(ctx.Args[0]):])
	}

	if strings.TrimSpace(textToSpeak) == "" {
		return ctx.Reply("Please provide text to convert to speech.")
	}

	if len(textToSpeak) > 500 {
		textToSpeak = textToSpeak[:500]
	}

	loader := ctx.StartLoader("Generating speech...")
	defer loader.Delete()

	mp3Data, err := fetchGoogleTTS(ctx.Ctx, textToSpeak, lang)
	if err != nil {
		slog.Error("handleTTS: Google TTS fetch failed", "err", err, "lang", lang)
		return ctx.Reply(fmt.Sprintf("Failed to generate speech audio: %v", err))
	}

	// Try converting MP3 to WhatsApp OGG Opus voice note using ffmpeg
	opusData, errConv := convertMP3ToOpus(ctx.Ctx, mp3Data)
	if errConv == nil && len(opusData) > 0 {
		return ctx.ReplyWithAudio(opusData, "audio/ogg; codecs=opus")
	}

	slog.Warn("handleTTS: ffmpeg OPUS conversion failed, falling back to MP3", "err", errConv)
	return ctx.ReplyWithAudio(mp3Data, "audio/mp4")
}

func fetchGoogleTTS(ctx context.Context, text string, lang string) ([]byte, error) {
	apiURL := fmt.Sprintf("https://translate.google.com/translate_tts?ie=UTF-8&q=%s&tl=%s&client=tw-ob", url.QueryEscape(text), url.QueryEscape(lang))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://translate.google.com/")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google TTS returned HTTP status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio response: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("received empty audio data")
	}

	return data, nil
}

func convertMP3ToOpus(ctx context.Context, mp3Bytes []byte) ([]byte, error) {
	tempDir := os.TempDir()
	tempMP3 := filepath.Join(tempDir, fmt.Sprintf("tts_%d.mp3", time.Now().UnixNano()))
	tempOpus := filepath.Join(tempDir, fmt.Sprintf("tts_%d.opus", time.Now().UnixNano()))

	if err := os.WriteFile(tempMP3, mp3Bytes, 0644); err != nil {
		return nil, err
	}
	defer os.Remove(tempMP3)
	defer os.Remove(tempOpus)

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", tempMP3, "-c:a", "libopus", "-b:a", "32k", "-application", "voip", tempOpus)
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return os.ReadFile(tempOpus)
}
