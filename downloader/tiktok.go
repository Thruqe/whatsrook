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
	tiktokIDRegex        = regexp.MustCompile(`/(?:video|photo)/(\d+)`)
	tiktokScriptRegex    = regexp.MustCompile(`<script id="__UNIVERSAL_DATA_FOR_REHYDRATION__" type="application/json">(.*?)</script>`)
	tiktokSIGIRegex      = regexp.MustCompile(`<script id="SIGI_STATE" type="application/json">(.*?)</script>`)
	tiktokEmbedPlayRegex = regexp.MustCompile(`"playAddr":"([^"]+)"`)
)

type tiktokRehydrationData struct {
	DefaultScope struct {
		WebappVideoDetail struct {
			StatusMsg string `json:"statusMsg"`
			ItemInfo  struct {
				ItemStruct struct {
					ID     string `json:"id"`
					Author struct {
						UniqueID string `json:"uniqueId"`
						Nickname string `json:"nickname"`
					} `json:"author"`
					Video struct {
						PlayAddr    string `json:"playAddr"`
						BitrateInfo []struct {
							CodecType string `json:"CodecType"`
							PlayAddr  struct {
								UrlList []string `json:"UrlList"`
							} `json:"PlayAddr"`
						} `json:"bitrateInfo"`
					} `json:"video"`
					Music struct {
						PlayURL string `json:"playUrl"`
						Title   string `json:"title"`
					} `json:"music"`
					ImagePost *struct {
						Images []struct {
							ImageURL struct {
								UrlList []string `json:"urlList"`
							} `json:"imageURL"`
						} `json:"images"`
					} `json:"imagePost"`
				} `json:"itemStruct"`
			} `json:"itemInfo"`
		} `json:"webapp.video-detail"`
	} `json:"__DEFAULT_SCOPE__"`
}

type tikWMResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		ID     string   `json:"id"`
		Title  string   `json:"title"`
		Play   string   `json:"play"`
		WmPlay string   `json:"wmplay"`
		Music  string   `json:"music"`
		Images []string `json:"images"`
		Author struct {
			UniqueID string `json:"unique_id"`
			Nickname string `json:"nickname"`
		} `json:"author"`
	} `json:"data"`
}

func (c *Client) fetchTikTokTikWM(ctx context.Context, rawURL string) (*Result, error) {
	apiURL := "https://www.tikwm.com/api/?url=" + url.QueryEscape(rawURL)
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
		return nil, fmt.Errorf("tikwm API status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var res tikWMResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}

	if res.Code != 0 || (res.Data.Play == "" && len(res.Data.Images) == 0) {
		return nil, fmt.Errorf("tikwm returned code %d: %s", res.Code, res.Msg)
	}

	var items []MediaItem
	author := res.Data.Author.UniqueID
	if author == "" {
		author = res.Data.Author.Nickname
	}

	if len(res.Data.Images) > 0 {
		for i, imgURL := range res.Data.Images {
			items = append(items, MediaItem{
				URL:      imgURL,
				Type:     "photo",
				Filename: fmt.Sprintf("tiktok_%s_%s_%d.jpg", author, res.Data.ID, i+1),
			})
		}
	} else if res.Data.Play != "" {
		videoURL := res.Data.Play
		if !strings.HasPrefix(videoURL, "http://") && !strings.HasPrefix(videoURL, "https://") {
			videoURL = "https://www.tikwm.com" + videoURL
		}
		items = append(items, MediaItem{
			URL:      videoURL,
			Type:     "video",
			Filename: fmt.Sprintf("tiktok_%s_%s.mp4", author, res.Data.ID),
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no media items returned by TikWM")
	}

	return &Result{
		Service: "tiktok",
		ID:      res.Data.ID,
		Author:  author,
		Title:   res.Data.Title,
		Items:   items,
		IsPhoto: len(res.Data.Images) > 0,
	}, nil
}

// DownloadTikTok extracts video, photo slides, or audio from a TikTok link.
func (c *Client) DownloadTikTok(ctx context.Context, rawURL string) (*Result, error) {
	// Strategy 1: High-performance TikWM API
	if tikResult, errTik := c.fetchTikTokTikWM(ctx, rawURL); errTik == nil && tikResult != nil && len(tikResult.Items) > 0 {
		return tikResult, nil
	}

	postID, err := c.resolveTikTokID(ctx, rawURL)
	if err != nil || postID == "" {
		postID = extractTikTokID(rawURL)
	}
	if postID == "" {
		return nil, fmt.Errorf("could not resolve TikTok post ID from URL: %s", rawURL)
	}

	// Strategy 2: Main web page extraction
	targetURL := fmt.Sprintf("https://www.tiktok.com/@i/video/%s", postID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", DefaultUserAgent)
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

		resp, errDo := c.HTTPClient.Do(req)
		if errDo == nil && resp.StatusCode == http.StatusOK {
			body, errRead := io.ReadAll(resp.Body)
			resp.Body.Close()
			if errRead == nil {
				html := string(body)

				// Strategy 2A: __UNIVERSAL_DATA_FOR_REHYDRATION__
				m := tiktokScriptRegex.FindStringSubmatch(html)
				if len(m) >= 2 {
					var rehydrated tiktokRehydrationData
					if errUnmarshal := json.Unmarshal([]byte(m[1]), &rehydrated); errUnmarshal == nil {
						detail := rehydrated.DefaultScope.WebappVideoDetail.ItemInfo.ItemStruct
						if detail.ID != "" {
							return parseTikTokDetail(detail, postID)
						}
					}
				}

				// Strategy 2B: SIGI_STATE
				if sigiMatch := tiktokSIGIRegex.FindStringSubmatch(html); len(sigiMatch) >= 2 {
					var sigi map[string]interface{}
					if json.Unmarshal([]byte(sigiMatch[1]), &sigi) == nil {
						if itemModule, ok := sigi["ItemModule"].(map[string]interface{}); ok {
							if itemMap, ok := itemModule[postID].(map[string]interface{}); ok {
								var items []MediaItem
								if videoMap, ok := itemMap["video"].(map[string]interface{}); ok {
									if playAddr, ok := videoMap["playAddr"].(string); ok && playAddr != "" {
										items = append(items, MediaItem{
											URL:      cleanEscapedURL(playAddr),
											Type:     "video",
											Filename: fmt.Sprintf("tiktok_%s.mp4", postID),
										})
										return &Result{
											Service: "tiktok",
											ID:      postID,
											Items:   items,
										}, nil
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Strategy 3: TikTok Embed API fallback (www.tiktok.com/embed/v2/postID)
	embedResult, errEmbed := c.fetchTikTokEmbed(ctx, postID)
	if errEmbed == nil && embedResult != nil && len(embedResult.Items) > 0 {
		return embedResult, nil
	}

	return nil, fmt.Errorf("tiktok extraction failed for post ID %s", postID)
}

func parseTikTokDetail(detail struct {
	ID     string `json:"id"`
	Author struct {
		UniqueID string `json:"uniqueId"`
		Nickname string `json:"nickname"`
	} `json:"author"`
	Video struct {
		PlayAddr    string `json:"playAddr"`
		BitrateInfo []struct {
			CodecType string `json:"CodecType"`
			PlayAddr  struct {
				UrlList []string `json:"UrlList"`
			} `json:"PlayAddr"`
		} `json:"bitrateInfo"`
	} `json:"video"`
	Music struct {
		PlayURL string `json:"playUrl"`
		Title   string `json:"title"`
	} `json:"music"`
	ImagePost *struct {
		Images []struct {
			ImageURL struct {
				UrlList []string `json:"urlList"`
			} `json:"imageURL"`
		} `json:"images"`
	} `json:"imagePost"`
}, postID string) (*Result, error) {
	var items []MediaItem
	isPhoto := false
	author := detail.Author.UniqueID
	if author == "" {
		author = detail.Author.Nickname
	}

	// 1. Photo Slides
	if detail.ImagePost != nil && len(detail.ImagePost.Images) > 0 {
		isPhoto = true
		for i, img := range detail.ImagePost.Images {
			var selectedURL string
			for _, uStr := range img.ImageURL.UrlList {
				if strings.Contains(uStr, ".jpeg") || strings.Contains(uStr, ".jpg") || strings.Contains(uStr, ".webp") {
					selectedURL = uStr
					break
				}
			}
			if selectedURL == "" && len(img.ImageURL.UrlList) > 0 {
				selectedURL = img.ImageURL.UrlList[0]
			}

			if selectedURL != "" {
				items = append(items, MediaItem{
					URL:      selectedURL,
					Type:     "photo",
					Filename: fmt.Sprintf("tiktok_%s_%s_%d.jpg", author, postID, i+1),
				})
			}
		}
	}

	// 2. Video
	if len(items) == 0 && detail.Video.PlayAddr != "" {
		videoURL := detail.Video.PlayAddr
		items = append(items, MediaItem{
			URL:      videoURL,
			Type:     "video",
			Filename: fmt.Sprintf("tiktok_%s_%s.mp4", author, postID),
		})
	}

	// 3. Audio Fallback
	if len(items) == 0 && detail.Music.PlayURL != "" {
		items = append(items, MediaItem{
			URL:      detail.Music.PlayURL,
			Type:     "audio",
			Filename: fmt.Sprintf("tiktok_%s_%s_audio.mp3", author, postID),
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no downloadable media found for TikTok post")
	}

	return &Result{
		Service: "tiktok",
		ID:      postID,
		Author:  author,
		Title:   detail.Music.Title,
		Items:   items,
		IsPhoto: isPhoto,
	}, nil
}

func (c *Client) fetchTikTokEmbed(ctx context.Context, postID string) (*Result, error) {
	embedURL := fmt.Sprintf("https://www.tiktok.com/embed/v2/%s", postID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
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
		return nil, fmt.Errorf("tiktok embed status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)
	m := tiktokEmbedPlayRegex.FindStringSubmatch(html)
	if len(m) >= 2 {
		videoURL := cleanEscapedURL(m[1])
		if videoURL != "" {
			return &Result{
				Service: "tiktok",
				ID:      postID,
				Items: []MediaItem{
					{
						URL:      videoURL,
						Type:     "video",
						Filename: fmt.Sprintf("tiktok_%s.mp4", postID),
					},
				},
			}, nil
		}
	}

	return nil, fmt.Errorf("no video found in tiktok embed page")
}

func (c *Client) resolveTikTokID(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)

	var finalURL string
	client := &http.Client{
		Timeout: c.HTTPClient.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			finalURL = req.URL.String()
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		if finalURL != "" {
			return extractTikTokID(finalURL), nil
		}
		return "", err
	}
	defer resp.Body.Close()

	if finalURL != "" {
		return extractTikTokID(finalURL), nil
	}
	return extractTikTokID(resp.Request.URL.String()), nil
}

func extractTikTokID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	m := tiktokIDRegex.FindStringSubmatch(u.Path)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}
