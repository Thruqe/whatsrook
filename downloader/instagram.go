package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ytDlpEntry struct {
	ID       string `json:"id"`
	Ext      string `json:"ext"`
	VCodec   string `json:"vcodec"`
	ACodec   string `json:"acodec"`
	URL      string `json:"url"`
	Filename string `json:"_filename"`
}

type ytDlpSingleJson struct {
	ID       string       `json:"id"`
	Title    string       `json:"title"`
	Uploader string       `json:"uploader"`
	Type     string       `json:"_type"` // "playlist" or "multi_video" for carousels
	Entries  []ytDlpEntry `json:"entries"`
	// Single media fields
	Ext    string `json:"ext"`
	VCodec string `json:"vcodec"`
	ACodec string `json:"acodec"`
}

// DownloadInstagram extracts single/multiple photos or videos using yt-dlp.
func (c *Client) DownloadInstagram(ctx context.Context, rawURL string) (*Result, error) {
	tmpDir := os.TempDir()
	outTemplate := filepath.Join(tmpDir, "ig_%(id)s_%(autonumber)s.%(ext)s")

	// 1. Dump full metadata structure
	metaArgs := []string{
		"--dump-single-json",
		"--no-warnings",
		rawURL,
	}

	cmdMeta := exec.CommandContext(ctx, "yt-dlp", metaArgs...)
	metaOut, err := cmdMeta.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("yt-dlp metadata failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("failed to run yt-dlp: %w", err)
	}

	var root ytDlpSingleJson
	if err := json.Unmarshal(metaOut, &root); err != nil {
		return nil, fmt.Errorf("failed to parse yt-dlp metadata: %w", err)
	}

	// 2. Execute download to disk
	dlArgs := []string{
		"-f", "b[ext=mp4]/bv*[ext=mp4]+ba[ext=m4a]/b",
		"-o", outTemplate,
		"--no-warnings",
		rawURL,
	}

	cmdDl := exec.CommandContext(ctx, "yt-dlp", dlArgs...)
	if dlOut, err := cmdDl.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("yt-dlp download failed: %s", string(dlOut))
	}

	var items []MediaItem
	hasVideo := false

	// Helper to categorize media
	parseMediaType := func(ext, vcodec string) (string, string) {
		ext = strings.ToLower(ext)
		if ext == "jpg" || ext == "jpeg" || ext == "png" || ext == "webp" || vcodec == "none" {
			return "photo", ext
		}
		return "video", "mp4"
	}

	// 3. Process Carousel (Multiple items)
	if len(root.Entries) > 0 {
		for i, entry := range root.Entries {
			mediaType, ext := parseMediaType(entry.Ext, entry.VCodec)
			if mediaType == "video" {
				hasVideo = true
			}

			// Locate downloaded file on disk
			localPath := filepath.Join(tmpDir, fmt.Sprintf("ig_%s_%05d.%s", root.ID, i+1, ext))
			if _, err := os.Stat(localPath); os.IsNotExist(err) {
				// Fallback glob if autonumber format differed
				matches, _ := filepath.Glob(filepath.Join(tmpDir, fmt.Sprintf("ig_%s_*", root.ID)))
				if len(matches) > i {
					localPath = matches[i]
				}
			}

			items = append(items, MediaItem{
				URL:      localPath,
				Type:     mediaType,
				Filename: fmt.Sprintf("instagram_%s_%d.%s", root.ID, i+1, ext),
			})
		}
	} else {
		// 4. Process Single Media (Single Reel, Post, or Video)
		mediaType, ext := parseMediaType(root.Ext, root.VCodec)
		if mediaType == "video" {
			hasVideo = true
		}

		localPath := filepath.Join(tmpDir, fmt.Sprintf("ig_%s_00001.%s", root.ID, ext))
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			matches, _ := filepath.Glob(filepath.Join(tmpDir, fmt.Sprintf("ig_%s_*", root.ID)))
			if len(matches) > 0 {
				localPath = matches[0]
			}
		}

		items = append(items, MediaItem{
			URL:      localPath,
			Type:     mediaType,
			Filename: fmt.Sprintf("instagram_%s.%s", root.ID, ext),
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no media items downloaded for URL: %s", rawURL)
	}

	return &Result{
		Service: "instagram",
		ID:      root.ID,
		Title:   root.Title,
		Author:  root.Uploader,
		IsPhoto: !hasVideo,
		Items:   items,
	}, nil
}
