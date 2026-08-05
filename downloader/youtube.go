package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
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
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"webpage_url"`
}

func (c *Client) runYtDlp(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)

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

	formatSpec := "best[ext=mp4]/best"
	mediaType := "video"
	ext := "mp4"

	if isAudioOnly {
		formatSpec = "bestaudio[ext=m4a]/bestaudio/best"
		mediaType = "audio"
		ext = "m4a"
	}

	out, err := c.runYtDlp(ctx, "--no-warnings",
		"--extractor-args", "youtube:player_client=android",
		"-f", formatSpec,
		"--print", "%(urls)s",
		"--print", "%(title)s",
		"--print", "%(uploader)s",
		"--print", "%(thumbnail)s",
		rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to extract YouTube media for video ID %s: %w", videoID, err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 4 {
		return nil, fmt.Errorf("unexpected yt-dlp output for video ID %s", videoID)
	}

	directURL := strings.TrimSpace(lines[0])
	title := strings.TrimSpace(lines[1])
	author := strings.TrimSpace(lines[2])
	thumbnail := strings.TrimSpace(lines[3])

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
