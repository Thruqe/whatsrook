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

	"whatsrook/logger"
)

var ytLog = logger.WhatsmeowStyle("Downloader/YouTube", "DEBUG", true)
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
			ytLog.Debugf("Using YouTube cookies file: %s", c.CookieFile)
			finalArgs = append(finalArgs, "--cookies", c.CookieFile)
		} else {
			ytLog.Warnf("CookieFile specified (%s) but file not found on disk", c.CookieFile)
		}
	} else {
		ytLog.Debugf("No CookieFile configured on downloader client")
	}

	finalArgs = append(finalArgs, args...)
	ytLog.Debugf("Executing yt-dlp binary with args: %s", strings.Join(finalArgs, " "))

	cmd := exec.CommandContext(ctx, "yt-dlp", finalArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	stderrStr := strings.TrimSpace(stderr.String())
	if stderrStr != "" {
		ytLog.Debugf("yt-dlp stderr output:\n%s", stderrStr)
	}

	if err != nil {
		ytLog.Warnf("yt-dlp command failed with err: %v | stderr: %s", err, stderrStr)
		return nil, fmt.Errorf("yt-dlp error: %w (stderr: %s)", err, stderrStr)
	}

	ytLog.Debugf("yt-dlp command executed successfully | stdout size: %d bytes", stdout.Len())
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
	ytLog.Debugf("Search called | query: %q | limit: %d | provider: %q | term: %q", query, limit, provider, searchTerm)

	out, err := c.runYtDlp(ctx, "-J", "--flat-playlist", "--no-warnings", searchTerm)
	if err != nil {
		ytLog.Errorf("Search yt-dlp execution failed: %v", err)
		return nil, err
	}

	var payload struct {
		Entries []SearchResult `json:"entries"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		ytLog.Errorf("Search JSON unmarshal failed: %v", err)
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	ytLog.Debugf("Search parsed %d video entries for query %q", len(payload.Entries), query)
	return payload.Entries, nil
}

func (c *Client) Info(ctx context.Context, mediaURL string) (*MediaInfo, error) {
	ytLog.Debugf("Info called for mediaURL: %s", mediaURL)

	out, err := c.runYtDlp(ctx, "-J", "--no-warnings", "--no-playlist", mediaURL)
	if err != nil {
		ytLog.Errorf("Info yt-dlp execution failed: %v", err)
		return nil, err
	}

	var info MediaInfo
	if err := json.Unmarshal(out, &info); err != nil {
		ytLog.Errorf("Info JSON unmarshal failed: %v", err)
		return nil, fmt.Errorf("failed to parse media info: %w", err)
	}

	ytLog.Debugf("Info successfully extracted | ID: %s | Title: %q | Uploader: %q | Duration: %.1fs", info.ID, info.Title, info.Uploader, info.Duration)
	return &info, nil
}

func (c *Client) DownloadYouTube(ctx context.Context, rawURL string) (*Result, error) {
	ytLog.Debugf("DownloadYouTube called for URL: %s -> delegating to DownloadYouTubeMedia (isAudioOnly=false)", rawURL)
	return c.DownloadYouTubeMedia(ctx, rawURL, false)
}

func (c *Client) DownloadYouTubeMedia(ctx context.Context, rawURL string, isAudioOnly bool) (*Result, error) {
	ytLog.Debugf("DownloadYouTubeMedia start | rawURL: %s | isAudioOnly: %v", rawURL, isAudioOnly)

	videoID := extractYouTubeID(rawURL)
	if videoID == "" {
		ytLog.Warnf("Could not extract video ID from rawURL: %s", rawURL)
		return nil, fmt.Errorf("could not extract video ID from URL: %s", rawURL)
	}
	ytLog.Debugf("Extracted YouTube video ID: %s", videoID)

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
	// Includes 'tv' client which bypasses YouTube datacenter IP blocks and SABR restrictions.
	var extractorArgSets [][]string
	if c.CookieFile != "" {
		if _, err := os.Stat(c.CookieFile); err == nil {
			ytLog.Debugf("Cookie file exists. Setting up extractorArgSets with tv client & cookie preference")
			extractorArgSets = [][]string{
				{"--extractor-args", "youtube:player_client=tv,mweb,android,web"},
				{"--extractor-args", "youtube:player_client=tv"},
				nil, // No custom extractor args (lets yt-dlp select optimal client with cookies)
				{"--extractor-args", "youtube:player_client=web,mweb,android"},
				{"--extractor-args", "youtube:player_client=android,web,mweb"},
			}
		} else {
			ytLog.Warnf("CookieFile set but file missing on disk. Using standard extractorArgSets")
			extractorArgSets = [][]string{
				{"--extractor-args", "youtube:player_client=tv,mweb,android,web"},
				{"--extractor-args", "youtube:player_client=tv"},
				{"--extractor-args", "youtube:player_client=android,web,mweb"},
				nil,
			}
		}
	} else {
		ytLog.Debugf("No cookies configured. Using default extractorArgSets with tv client first")
		extractorArgSets = [][]string{
			{"--extractor-args", "youtube:player_client=tv,mweb,android,web"},
			{"--extractor-args", "youtube:player_client=tv"},
			{"--extractor-args", "youtube:player_client=android,web,mweb"},
			nil,
		}
	}

	var lastErr error
	var out []byte

	// Multi-level retry across extractor args and format specs
	ytLog.Debugf("Starting multi-level extraction loop for video ID %s (%d extractorArgSets x %d formatSpecs)", videoID, len(extractorArgSets), len(formatSpecs))

	for i, extArgs := range extractorArgSets {
		for j, formatSpec := range formatSpecs {
			ytLog.Debugf("[Attempt Set %d.%d] Trying extArgs: %v | formatSpec: %q", i+1, j+1, extArgs, formatSpec)

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
					ytLog.Debugf("[Attempt Set %d.%d SUCCESS] Extracted direct stream URL: %s", i+1, j+1, strings.TrimSpace(lines[0]))
					break
				}
			} else {
				ytLog.Debugf("[Attempt Set %d.%d FAILED] err: %v", i+1, j+1, lastErr)
			}
		}
		if lastErr == nil && len(bytes.TrimSpace(out)) > 0 {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) >= 1 && strings.TrimSpace(lines[0]) != "" {
				ytLog.Debugf("Breaking outer loop on Set %d success", i+1)
				break
			}
		}
	}

	// Final fallback: try yt-dlp without -f format selection or extractor args
	if lastErr != nil || len(bytes.TrimSpace(out)) == 0 {
		ytLog.Warnf("All extractorArg and formatSpec attempts failed for video ID %s. Executing final fallback yt-dlp call without -f or --extractor-args...", videoID)
		out, lastErr = c.runYtDlp(ctx, "--no-warnings",
			"--print", "%(urls)s",
			"--print", "%(title)s",
			"--print", "%(uploader)s",
			"--print", "%(thumbnail)s",
			rawURL,
		)
	}

	if lastErr != nil {
		ytLog.Errorf("Failed to extract YouTube media for video ID %s after all attempts: %v", videoID, lastErr)
		return nil, fmt.Errorf("failed to extract YouTube media for video ID %s: %w", videoID, lastErr)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 1 || strings.TrimSpace(lines[0]) == "" {
		ytLog.Errorf("Unexpected empty yt-dlp output for video ID %s", videoID)
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
		ytLog.Errorf("Empty direct stream URL returned from yt-dlp for video ID %s", videoID)
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

	ytLog.Debugf("DownloadYouTubeMedia completed successfully for ID %s | Title: %q | Author: %q | DirectURL: %s", videoID, title, author, directURL)

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
		ytLog.Debugf("extractYouTubeID failed to parse rawURL %q: %v", rawURL, err)
		return ""
	}

	if u.Host == "youtu.be" {
		id := strings.TrimPrefix(u.Path, "/")
		ytLog.Debugf("extractYouTubeID matched short URL host youtu.be -> ID: %s", id)
		return id
	}

	if m := ytIDRegex.FindStringSubmatch(rawURL); len(m) > 1 {
		ytLog.Debugf("extractYouTubeID matched regex -> ID: %s", m[1])
		return m[1]
	}

	id := u.Query().Get("v")
	ytLog.Debugf("extractYouTubeID matched query param 'v' -> ID: %s", id)
	return id
}
