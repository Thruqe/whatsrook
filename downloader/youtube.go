package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var ytIDRegex = regexp.MustCompile(`(?:v=|/v/|embed/|shorts/|youtu\.be/)([a-zA-Z0-9_-]{11})`)

type MediaInfo struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Uploader   string  `json:"uploader"`
	Duration   float64 `json:"duration"`
	Thumbnail  string  `json:"thumbnail"`
	WebpageURL string  `json:"webpage_url"`
}

type SearchResult struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Uploader   string  `json:"uploader"`
	Duration   float64 `json:"duration"`
	URL        string  `json:"url"`
	WebpageURL string  `json:"webpage_url"`
	ViewCount  int64   `json:"view_count"`
}

func (s *SearchResult) FormatDuration() string {
	if s.Duration <= 0 {
		return "N/A"
	}
	totalSeconds := int(s.Duration)
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	hours := minutes / 60
	minutes = minutes % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

func (s *SearchResult) GetURL() string {
	if s.WebpageURL != "" && strings.HasPrefix(s.WebpageURL, "http") {
		return s.WebpageURL
	}
	if s.URL != "" && strings.HasPrefix(s.URL, "http") {
		return s.URL
	}
	if s.ID != "" {
		return "https://www.youtube.com/watch?v=" + s.ID
	}
	if s.URL != "" {
		return "https://www.youtube.com/watch?v=" + s.URL
	}
	return ""
}

func (c *Client) runYtDlp(ctx context.Context, args ...string) ([]byte, error) {
	var finalArgs []string
	if c.CookieFile != "" {
		if _, err := os.Stat(c.CookieFile); err == nil {
			finalArgs = append(finalArgs, "--cookies", c.CookieFile)
		}
	}
	finalArgs = append(finalArgs, args...)

	cmd := exec.CommandContext(ctx, "yt-dlp", finalArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("yt-dlp error: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

func (c *Client) Search(ctx context.Context, query string, limit int, provider string) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	if provider == "" {
		provider = "ytsearch"
	}

	searchTerm := fmt.Sprintf("%s%d:%s", provider, limit, query)

	out, err := c.runYtDlp(ctx, "-J", "--flat-playlist", "--no-warnings", searchTerm)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Entries []SearchResult `json:"entries"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	return payload.Entries, nil
}

func (c *Client) Info(ctx context.Context, mediaURL string) (*MediaInfo, error) {
	out, err := c.runYtDlp(ctx, "-J", "--no-warnings", "--no-playlist", mediaURL)
	if err != nil {
		return nil, err
	}

	var info MediaInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("failed to parse media info: %w", err)
	}

	return &info, nil
}

func (c *Client) DownloadYouTube(ctx context.Context, rawURL string) (*Result, error) {
	return c.DownloadYouTubeMedia(ctx, rawURL, false)
}

func (c *Client) DownloadYouTubeMedia(ctx context.Context, rawURL string, isAudioOnly bool) (*Result, error) {
	videoID := extractYouTubeID(rawURL)
	if videoID == "" {
		return nil, fmt.Errorf("could not extract video ID from URL: %s", rawURL)
	}

	mediaType := "video"
	ext := "mp4"
	if isAudioOnly {
		mediaType = "audio"
		ext = "opus"
	}

	// Define format specifications in order of preference
	var formatSpecs []string
	if isAudioOnly {
		formatSpecs = []string{
			"bestaudio[acodec=opus]/bestaudio[ext=webm]/bestaudio/best/ba/b",
			"bestaudio/best",
			"ba/b",
			"ba*/b*",
			"best",
		}
	} else {
		formatSpecs = []string{
			"bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best/bv*+ba*/b*",
			"best[ext=mp4]/best",
			"b/bv*+ba*",
			"best",
		}
	}

	// Extractor args configurations:
	// If cookies are set, default web/mweb clients work best without forcing android client alone.
	var extractorArgSets [][]string
	if c.CookieFile != "" {
		if _, err := os.Stat(c.CookieFile); err == nil {
			extractorArgSets = [][]string{
				nil, // No custom extractor args (lets yt-dlp select optimal client with cookies)
				{"--extractor-args", "youtube:player_client=web,mweb,android"},
				{"--extractor-args", "youtube:player_client=android,web,mweb"},
			}
		} else {
			extractorArgSets = [][]string{
				{"--extractor-args", "youtube:player_client=android,web,mweb"},
				nil,
			}
		}
	} else {
		extractorArgSets = [][]string{
			{"--extractor-args", "youtube:player_client=android,web,mweb"},
			nil,
		}
	}

	var lastErr error
	var out []byte

	// Multi-level retry across extractor args and format specs
	for _, extArgs := range extractorArgSets {
		for _, formatSpec := range formatSpecs {
			args := []string{"--no-warnings"}
			if len(extArgs) > 0 {
				args = append(args, extArgs...)
			}
			args = append(args,
				"-f", formatSpec,
				"--print", "%(urls)s",
				"--print", "%(title)s",
				"--print", "%(uploader)s",
				"--print", "%(thumbnail)s",
				rawURL,
			)

			out, lastErr = c.runYtDlp(ctx, args...)
			if lastErr == nil && len(bytes.TrimSpace(out)) > 0 {
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				if len(lines) >= 1 && strings.TrimSpace(lines[0]) != "" {
					break
				}
			}
		}
		if lastErr == nil && len(bytes.TrimSpace(out)) > 0 {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) >= 1 && strings.TrimSpace(lines[0]) != "" {
				break
			}
		}
	}

	// Final fallback: try yt-dlp without -f format selection or extractor args
	if lastErr != nil || len(bytes.TrimSpace(out)) == 0 {
		out, lastErr = c.runYtDlp(ctx, "--no-warnings",
			"--print", "%(urls)s",
			"--print", "%(title)s",
			"--print", "%(uploader)s",
			"--print", "%(thumbnail)s",
			rawURL,
		)
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to extract YouTube media for video ID %s: %w", videoID, lastErr)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 1 || strings.TrimSpace(lines[0]) == "" {
		return nil, fmt.Errorf("unexpected yt-dlp output for video ID %s", videoID)
	}

	directURL := strings.TrimSpace(lines[0])
	title := ""
	if len(lines) > 1 {
		title = strings.TrimSpace(lines[1])
	}
	author := ""
	if len(lines) > 2 {
		author = strings.TrimSpace(lines[2])
	}
	thumbnail := ""
	if len(lines) > 3 {
		thumbnail = strings.TrimSpace(lines[3])
	}

	if directURL == "" {
		return nil, fmt.Errorf("empty stream URL from yt-dlp for video ID %s", videoID)
	}
	if title == "" {
		title = fmt.Sprintf("YouTube %s (%s)", mediaType, videoID)
	}
	if author == "NA" {
		author = ""
	}
	if thumbnail == "NA" {
		thumbnail = ""
	}

	return &Result{
		Service:   "youtube",
		ID:        videoID,
		Title:     title,
		Author:    author,
		Thumbnail: thumbnail,
		Items: []MediaItem{
			{
				URL:      directURL,
				Type:     mediaType,
				Filename: fmt.Sprintf("youtube_%s.%s", videoID, ext),
			},
		},
	}, nil
}

func extractYouTubeID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	if u.Host == "youtu.be" {
		return strings.TrimPrefix(u.Path, "/")
	}

	if m := ytIDRegex.FindStringSubmatch(rawURL); len(m) > 1 {
		return m[1]
	}

	return u.Query().Get("v")
}
