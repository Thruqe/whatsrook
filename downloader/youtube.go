package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"whatsrook/utils"
)

var ytLog = utils.WhatsmeowStyle("Downloader/YouTube", "DEBUG", true)
var ytIDRegex = regexp.MustCompile(`(?:v=|/v/|embed/|shorts/|youtu\.be/)([a-zA-Z0-9_-]{11})`)

const fallbackCookiesURL = "https://gist.githubusercontent.com/Thruqe/e43b6b98dd75f4f5e31bc319365f05e0/raw/dbe5901ecb7586d29684217d6645a29e617f34f0/cookies.txt"

var (
	fallbackCookieOnce sync.Once
	fallbackCookiePath string // empty string means fetch failed / unavailable
)

// getFallbackCookieFile lazily fetches and caches a generic fallback cookies file
// to disk, for use only when no CookieFile is configured on the client.
// If the fetch fails for any reason, it returns "" and callers should proceed
// without cookies (surfacing the normal bot-detection error downstream).
func getFallbackCookieFile(ctx context.Context) string {
	fallbackCookieOnce.Do(func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fallbackCookiesURL, nil)
		if err != nil {
			ytLog.Debugf("Fallback cookies: failed to build request: %v", err)
			return
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			ytLog.Debugf("Fallback cookies: fetch failed: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			ytLog.Debugf("Fallback cookies: unexpected status %d", resp.StatusCode)
			return
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // cap at 1MB, sanity limit
		if err != nil {
			ytLog.Debugf("Fallback cookies: read failed: %v", err)
			return
		}

		trimmed := bytes.TrimSpace(body)
		if len(trimmed) == 0 {
			ytLog.Debugf("Fallback cookies: empty response body")
			return
		}

		tmpPath := filepath.Join(os.TempDir(), "whatsrook_fallback_cookies.txt")
		if err := os.WriteFile(tmpPath, trimmed, 0o600); err != nil {
			ytLog.Debugf("Fallback cookies: failed to write temp file: %v", err)
			return
		}

		ytLog.Debugf("Fallback cookies: cached to %s (%d bytes)", tmpPath, len(trimmed))
		fallbackCookiePath = tmpPath
	})

	return fallbackCookiePath
}

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
	binPath, err := GetYTDLPPath(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to locate yt-dlp binary: %w", err)
	}

	var finalArgs []string
	finalArgs = append(finalArgs, "--no-check-certificates", "--legacy-server-connect")

	cookieFile := c.CookieFile
	if cookieFile != "" {
		if _, err := os.Stat(cookieFile); err == nil {
			ytLog.Debugf("Using YouTube cookies file: %s", cookieFile)
			finalArgs = append(finalArgs, "--cookies", cookieFile)
		} else {
			ytLog.Warnf("CookieFile specified (%s) but file not found on disk", cookieFile)
			cookieFile = ""
		}
	}

	if cookieFile == "" {
		ytLog.Debugf("No CookieFile configured on downloader client; attempting fallback cookies")
		if fbPath := getFallbackCookieFile(ctx); fbPath != "" {
			ytLog.Debugf("Using fallback cookies file: %s", fbPath)
			finalArgs = append(finalArgs, "--cookies", fbPath)
		} else {
			ytLog.Debugf("Fallback cookies unavailable; proceeding without cookies")
		}
	}

	finalArgs = append(finalArgs, args...)
	ytLog.Debugf("Executing yt-dlp binary (%s) with args: %s", binPath, strings.Join(finalArgs, " "))

	return executeYtDlpWithRetry(ctx, binPath, finalArgs)
}

func executeYtDlpWithRetry(ctx context.Context, binPath string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binPath, args...)
	setSSLBypassEnv(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "permission denied") {
		// Non-root permission fallback: run zipapp binary via python3/python without requiring root/chmod permissions
		pyExec := "python3"
		if _, pErr := exec.LookPath("python3"); pErr != nil {
			pyExec = "python"
		}
		pyArgs := append([]string{binPath}, args...)
		cmd = exec.CommandContext(ctx, pyExec, pyArgs...)
		setSSLBypassEnv(cmd)
		stdout.Reset()
		stderr.Reset()
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
	}

	stderrStr := strings.TrimSpace(stderr.String())
	if stderrStr != "" {
		ytLog.Debugf("yt-dlp stderr output:\n%s", stderrStr)
	}

	if err != nil {
		if strings.Contains(stderrStr, "CERTIFICATE_VERIFY_FAILED") || strings.Contains(stderrStr, "Unable to download API page") || strings.Contains(stderrStr, "Please report this issue") {
			ytLog.Warnf("yt-dlp execution failed with SSL/API error. Auto-updating yt-dlp binary and retrying...")
			_ = UpdateYTDLP(ctx)

			newPath, _ := GetYTDLPPath(ctx)
			retryCmd := exec.CommandContext(ctx, newPath, args...)
			setSSLBypassEnv(retryCmd)

			var retryStdout, retryStderr bytes.Buffer
			retryCmd.Stdout = &retryStdout
			retryCmd.Stderr = &retryStderr

			retryErr := retryCmd.Run()
			if retryErr != nil && strings.Contains(strings.ToLower(retryErr.Error()), "permission denied") {
				pyExec := "python3"
				if _, pErr := exec.LookPath("python3"); pErr != nil {
					pyExec = "python"
				}
				pyArgs := append([]string{newPath}, args...)
				retryCmd = exec.CommandContext(ctx, pyExec, pyArgs...)
				setSSLBypassEnv(retryCmd)
				retryStdout.Reset()
				retryStderr.Reset()
				retryCmd.Stdout = &retryStdout
				retryCmd.Stderr = &retryStderr
				retryErr = retryCmd.Run()
			}

			retryStderrStr := strings.TrimSpace(retryStderr.String())
			if retryErr != nil {
				return nil, fmt.Errorf("yt-dlp error after auto-update: %w (stderr: %s)", retryErr, retryStderrStr)
			}
			return retryStdout.Bytes(), nil
		}

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
			"ba/b",
		}
	} else {
		formatSpecs = []string{
			"bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best/bv*+ba*/b*",
			"b",
		}
	}

	// Extractor args configurations:
	// Includes 'tv' client which bypasses YouTube datacenter IP blocks and SABR restrictions.
	var extractorArgSets [][]string
	if c.CookieFile != "" {
		if _, err := os.Stat(c.CookieFile); err == nil {
			ytLog.Debugf("Cookie file exists (%s). Setting up extractorArgSets with tv client & cookie preference", c.CookieFile)
			extractorArgSets = [][]string{
				{"--extractor-args", "youtube:player_client=tv,mweb,android,web"},
				{"--extractor-args", "youtube:player_client=tv"},
				nil, // No custom extractor args (lets yt-dlp select optimal client with cookies)
				{"--extractor-args", "youtube:player_client=web,mweb,android"},
				{"--extractor-args", "youtube:player_client=android,web,mweb"},
			}
		} else {
			ytLog.Warnf("CookieFile set (%s) but file missing on disk. Using standard extractorArgSets", c.CookieFile)
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

	stopLoop := false
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
					stopLoop = true
					break
				}
			} else {
				ytLog.Debugf("[Attempt Set %d.%d FAILED] err: %v", i+1, j+1, lastErr)
				if lastErr != nil && IsBotDetectionError(lastErr) {
					ytLog.Warnf("Detected YouTube bot detection block. Stopping retries immediately: %v", lastErr)
					stopLoop = true
					break
				}
			}
		}
		if stopLoop {
			break
		}
	}

	// Final fallback: try yt-dlp without -f format selection or extractor args ONLY if not bot detection blocked
	if (lastErr != nil || len(bytes.TrimSpace(out)) == 0) && !IsBotDetectionError(lastErr) {
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
		ytLog.Errorf("Failed to extract YouTube media for video ID %s: %v", videoID, lastErr)
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

// IsBotDetectionError checks if an error indicates YouTube bot detection or login requirements.
func IsBotDetectionError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "sign in to confirm") ||
		strings.Contains(lower, "bot") ||
		strings.Contains(lower, "captcha") ||
		strings.Contains(lower, "use --cookies") ||
		strings.Contains(lower, "exporting youtube cookies")
}
