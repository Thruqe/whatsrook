package downloader

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var twitterStatusRegex = regexp.MustCompile(`/(?:status|statuses)/(\d+)`)

type twitterSyndicationResponse struct {
	Text string `json:"text"`
	User struct {
		Name       string `json:"name"`
		ScreenName string `json:"screen_name"`
	} `json:"user"`
	MediaDetails []struct {
		Type          string `json:"type"`
		MediaURLHTTPS string `json:"media_url_https"`
		VideoInfo     *struct {
			Variants []struct {
				Bitrate     int    `json:"bitrate"`
				ContentType string `json:"content_type"`
				URL         string `json:"url"`
			} `json:"variants"`
		} `json:"video_info"`
	} `json:"mediaDetails"`
}

// DownloadTwitter extracts media from a Twitter / X post link.
func (c *Client) DownloadTwitter(ctx context.Context, rawURL string) (*Result, error) {
	tweetID := extractTwitterID(rawURL)
	if tweetID == "" {
		return nil, fmt.Errorf("could not parse tweet ID from URL: %s", rawURL)
	}

	canonicalURL := fmt.Sprintf("https://twitter.com/i/status/%s", tweetID)

	// Strategy 1: Syndication API
	res, err := c.fetchTwitterSyndication(ctx, tweetID)
	if err == nil && res != nil && len(res.Items) > 0 {
		return res, nil
	}
	mainLog.Debugf("Syndication API failed for tweet %s: %v", tweetID, err)

	// Strategy 2: Twitsave extractor
	resTwit, errTwit := c.fetchTwitterTwitsave(ctx, tweetID)
	if errTwit == nil && resTwit != nil && len(resTwit.Items) > 0 {
		return resTwit, nil
	}
	mainLog.Debugf("Twitsave extractor failed for tweet %s: %v", tweetID, errTwit)

	// Strategy 3: SSSTwitter fallback
	resSSS, errSSS := c.fetchTwitterSSSTwitter(ctx, canonicalURL, tweetID)
	if errSSS == nil && resSSS != nil && len(resSSS.Items) > 0 {
		return resSSS, nil
	}
	mainLog.Debugf("SSSTwitter fallback failed for tweet %s: %v", tweetID, errSSS)

	return nil, fmt.Errorf("failed to extract media for tweet ID %s", tweetID)
}

func (c *Client) fetchTwitterSyndication(ctx context.Context, tweetID string) (*Result, error) {
	token := generateTwitterToken(tweetID)
	syndicationURL := fmt.Sprintf("https://cdn.syndication.twimg.com/tweet-result?id=%s&token=%s", tweetID, token)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, syndicationURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("syndication HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data twitterSyndicationResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse syndication JSON: %w", err)
	}

	if len(data.MediaDetails) == 0 {
		return nil, fmt.Errorf("no media details found in tweet")
	}

	var items []MediaItem
	isPhoto := true

	for i, media := range data.MediaDetails {
		switch media.Type {
		case "photo":
			photoURL := media.MediaURLHTTPS
			if !strings.Contains(photoURL, "?") {
				photoURL += "?name=orig"
			}
			items = append(items, MediaItem{
				URL:      photoURL,
				Type:     "photo",
				Filename: fmt.Sprintf("twitter_%s_%d.jpg", tweetID, i+1),
			})

		case "video", "animated_gif":
			isPhoto = false
			if media.VideoInfo != nil && len(media.VideoInfo.Variants) > 0 {
				var bestURL string
				maxBitrate := -1
				for _, variant := range media.VideoInfo.Variants {
					if variant.ContentType == "video/mp4" {
						if variant.Bitrate > maxBitrate || bestURL == "" {
							maxBitrate = variant.Bitrate
							bestURL = variant.URL
						}
					}
				}

				if bestURL != "" {
					// Clean ?tag= from URL
					if u, err := url.Parse(bestURL); err == nil {
						q := u.Query()
						q.Del("tag")
						u.RawQuery = q.Encode()
						bestURL = u.String()
					}

					itemType := "video"
					ext := "mp4"
					if media.Type == "animated_gif" {
						itemType = "gif"
					}

					items = append(items, MediaItem{
						URL:      bestURL,
						Type:     itemType,
						Filename: fmt.Sprintf("twitter_%s_%d.%s", tweetID, i+1, ext),
						ThumbURL: media.MediaURLHTTPS,
					})
				}
			}
		}
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no valid media items extracted from tweet")
	}

	author := data.User.ScreenName
	if author == "" {
		author = data.User.Name
	}

	return &Result{
		Service: "twitter",
		ID:      tweetID,
		Title:   data.Text,
		Author:  author,
		Items:   items,
		IsPhoto: isPhoto,
	}, nil
}

func (c *Client) fetchTwitterTwitsave(ctx context.Context, tweetID string) (*Result, error) {
	canonicalURL := fmt.Sprintf("https://twitter.com/i/status/%s", tweetID)
	apiURL := "https://twitsave.com/info?url=" + url.QueryEscape(canonicalURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitsave HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)

	var items []MediaItem
	seen := make(map[string]bool)

	// 1. Direct <video src="...">
	videoSrcRegex := regexp.MustCompile(`<video[^>]*src="([^"]+)"`)
	m := videoSrcRegex.FindStringSubmatch(html)
	if len(m) > 1 && strings.HasPrefix(m[1], "http") {
		uStr := m[1]
		seen[uStr] = true
		items = append(items, MediaItem{
			URL:      uStr,
			Type:     "video",
			Filename: fmt.Sprintf("twitter_%s.mp4", tweetID),
		})
	}

	// 2. Base64 encoded download links: href="https://twitsave.com/download?file=..."
	dlRegex := regexp.MustCompile(`href="https://twitsave\.com/download\?file=([^"]+)"`)
	matches := dlRegex.FindAllStringSubmatch(html, -1)
	for i, match := range matches {
		if len(match) > 1 {
			decodedBytes, errDec := base64.StdEncoding.DecodeString(match[1])
			if errDec == nil {
				decodedURL := string(decodedBytes)
				if strings.HasPrefix(decodedURL, "http") && !seen[decodedURL] {
					seen[decodedURL] = true
					items = append(items, MediaItem{
						URL:      decodedURL,
						Type:     "video",
						Filename: fmt.Sprintf("twitter_%s_%d.mp4", tweetID, i+1),
					})
				}
			}
		}
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no media items found in twitsave response")
	}

	return &Result{
		Service: "twitter",
		ID:      tweetID,
		Items:   items,
	}, nil
}

func (c *Client) fetchTwitterSSSTwitter(ctx context.Context, rawURL, tweetID string) (*Result, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Jar:     jar,
		Timeout: c.HTTPClient.Timeout,
	}

	formData := url.Values{}
	formData.Set("id", rawURL)
	formData.Set("locale", "en")
	formData.Set("source", "form")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://ssstwitter.com/", strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("hx-request", "true")
	req.Header.Set("hx-target", "target")
	req.Header.Set("hx-current-url", "https://ssstwitter.com/")
	req.Header.Set("origin", "https://ssstwitter.com")
	req.Header.Set("referer", "https://ssstwitter.com/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ssstwitter HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)

	// Extract direct download URLs or href download links
	var items []MediaItem
	hrefRegex := regexp.MustCompile(`(?:href|data-directurl)="([^"]+)"`)
	matches := hrefRegex.FindAllStringSubmatch(html, -1)

	seen := make(map[string]bool)
	for i, m := range matches {
		if len(m) < 2 {
			continue
		}
		dlURL := m[1]
		if dlURL == "" || seen[dlURL] {
			continue
		}
		if !strings.HasPrefix(dlURL, "http://") && !strings.HasPrefix(dlURL, "https://") {
			continue
		}
		if strings.Contains(dlURL, "ssstwitter.com") || strings.Contains(dlURL, "reelsvideo.io") ||
			strings.Contains(dlURL, "ssstik.io") || strings.Contains(dlURL, "getmyfb.com") ||
			strings.Contains(dlURL, "googlesyndication.com") || strings.Contains(dlURL, "google.com") ||
			strings.Contains(dlURL, "play.google.com") {
			continue
		}
		seen[dlURL] = true

		itemType := "video"
		ext := "mp4"
		if strings.Contains(dlURL, ".jpg") || strings.Contains(dlURL, ".png") || strings.Contains(dlURL, ".webp") {
			itemType = "photo"
			ext = "jpg"
		}

		items = append(items, MediaItem{
			URL:      dlURL,
			Type:     itemType,
			Filename: fmt.Sprintf("twitter_%s_%d.%s", tweetID, i+1, ext),
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no downloadable links found in ssstwitter response")
	}

	return &Result{
		Service: "twitter",
		ID:      tweetID,
		Items:   items,
	}, nil
}

func generateTwitterToken(tweetID string) string {
	idNum, err := strconv.ParseFloat(tweetID, 64)
	if err != nil {
		return "token"
	}
	val := (idNum / 1e15) * math.Pi
	str := strconv.FormatFloat(val, 'f', -1, 64)
	// Base36 encoding approximation match with cobalt
	str = strings.ReplaceAll(str, ".", "")
	str = strings.TrimLeft(str, "0")
	if len(str) > 8 {
		return str[:8]
	}
	return str
}

func extractTwitterID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	m := twitterStatusRegex.FindStringSubmatch(u.Path)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}
