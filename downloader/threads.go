package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var (
	threadsPostRegex = regexp.MustCompile(`/@([^/]+)/post/([^/?#]+)`)
	threadsSJSRegex  = regexp.MustCompile(`<script type="application/json"[^>]*\bdata-sjs\b[^>]*>(.*?)</script>`)
)

type threadsPost struct {
	Code          string `json:"code"`
	VideoVersions []struct {
		URL string `json:"url"`
	} `json:"video_versions"`
	ImageVersions2 struct {
		Candidates []struct {
			URL string `json:"url"`
		} `json:"candidates"`
	} `json:"image_versions2"`
	CarouselMedia []struct {
		VideoVersions []struct {
			URL string `json:"url"`
		} `json:"video_versions"`
		ImageVersions2 struct {
			Candidates []struct {
				URL string `json:"url"`
			} `json:"candidates"`
		} `json:"image_versions2"`
	} `json:"carousel_media"`
}

// DownloadThreads extracts media from a Threads post link.
func (c *Client) DownloadThreads(ctx context.Context, rawURL string) (*Result, error) {
	resolvedURL, err := c.resolveRedirect(ctx, rawURL)
	if err != nil {
		resolvedURL = rawURL
	}

	user, postID := extractThreadsUserAndPostID(resolvedURL)
	if postID == "" {
		return nil, fmt.Errorf("could not parse Threads post ID from URL: %s", rawURL)
	}

	targetURL := fmt.Sprintf("https://www.threads.com/@%s/post/%s/", user, postID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Ch-Ua", `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("threads request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("threads HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)
	matches := threadsSJSRegex.FindAllStringSubmatch(html, -1)
	var foundPost *threadsPost

	for _, m := range matches {
		if len(m) > 1 {
			var rawData interface{}
			if json.Unmarshal([]byte(m[1]), &rawData) == nil {
				if post := findThreadsPostInNode(rawData, postID); post != nil {
					foundPost = post
					break
				}
			}
		}
	}

	if foundPost == nil {
		return nil, fmt.Errorf("could not find post data for Threads ID %s", postID)
	}

	var items []MediaItem
	isPhoto := true

	if len(foundPost.CarouselMedia) > 0 {
		for i, cm := range foundPost.CarouselMedia {
			if len(cm.VideoVersions) > 0 && cm.VideoVersions[0].URL != "" {
				isPhoto = false
				items = append(items, MediaItem{
					URL:      cm.VideoVersions[0].URL,
					Type:     "video",
					Filename: fmt.Sprintf("threads_%s_%d.mp4", postID, i+1),
				})
			} else if len(cm.ImageVersions2.Candidates) > 0 && cm.ImageVersions2.Candidates[0].URL != "" {
				items = append(items, MediaItem{
					URL:      cm.ImageVersions2.Candidates[0].URL,
					Type:     "photo",
					Filename: fmt.Sprintf("threads_%s_%d.jpg", postID, i+1),
				})
			}
		}
	} else if len(foundPost.VideoVersions) > 0 && foundPost.VideoVersions[0].URL != "" {
		isPhoto = false
		items = append(items, MediaItem{
			URL:      foundPost.VideoVersions[0].URL,
			Type:     "video",
			Filename: fmt.Sprintf("threads_%s.mp4", postID),
		})
	} else if len(foundPost.ImageVersions2.Candidates) > 0 && foundPost.ImageVersions2.Candidates[0].URL != "" {
		items = append(items, MediaItem{
			URL:      foundPost.ImageVersions2.Candidates[0].URL,
			Type:     "photo",
			Filename: fmt.Sprintf("threads_%s.jpg", postID),
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no media items found in Threads post")
	}

	return &Result{
		Service: "threads",
		ID:      postID,
		Author:  user,
		Items:   items,
		IsPhoto: isPhoto,
	}, nil
}

func findThreadsPostInNode(node interface{}, targetCode string) *threadsPost {
	switch v := node.(type) {
	case map[string]interface{}:
		if items, ok := v["thread_items"].([]interface{}); ok {
			for _, item := range items {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if postObj, ok := itemMap["post"]; ok {
						data, _ := json.Marshal(postObj)
						var p threadsPost
						if json.Unmarshal(data, &p) == nil && p.Code == targetCode {
							return &p
						}
					}
				}
			}
		}
		for _, child := range v {
			if res := findThreadsPostInNode(child, targetCode); res != nil {
				return res
			}
		}
	case []interface{}:
		for _, elem := range v {
			if res := findThreadsPostInNode(elem, targetCode); res != nil {
				return res
			}
		}
	}
	return nil
}

func extractThreadsUserAndPostID(rawURL string) (string, string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", ""
	}
	m := threadsPostRegex.FindStringSubmatch(u.Path)
	if len(m) > 2 {
		return m[1], m[2]
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 {
		return "user", parts[len(parts)-1]
	}
	return "", ""
}
