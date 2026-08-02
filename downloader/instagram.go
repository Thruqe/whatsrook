package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
)

var (
	igShortcodeRegex = regexp.MustCompile(`/(?:p|reel|reels|tv)/([^/?#]+)`)
	igEmbedJSONRegex = regexp.MustCompile(`"init",\[\],\[(.*?)\]\],`)
)

type igEmbedData struct {
	ContextJSON string `json:"contextJSON"`
}

type igMediaNode struct {
	DisplayURL string     `json:"display_url"`
	VideoURL   string     `json:"video_url"`
	IsVideo    bool       `json:"is_video"`
	TypeName   string     `json:"__typename"`
	Sidecar    *igSidecar `json:"edge_sidecar_to_children"`
}

type igSidecar struct {
	Edges []struct {
		Node igMediaNode `json:"node"`
	} `json:"edges"`
}

type igShortcodeMedia struct {
	ShortcodeMedia igMediaNode `json:"shortcode_media"`
}

// DownloadInstagram extracts media from an Instagram post, reel, or TV link.
func (c *Client) DownloadInstagram(ctx context.Context, rawURL string) (*Result, error) {
	shortcode := extractInstagramShortcode(rawURL)
	if shortcode == "" {
		return nil, fmt.Errorf("could not parse shortcode from Instagram URL: %s", rawURL)
	}

	// Try Embed API method first
	res, err := c.fetchInstagramEmbed(ctx, shortcode)
	if err == nil && res != nil && len(res.Items) > 0 {
		return res, nil
	}

	// Try OEmbed / Mobile API fallback method
	res, err = c.fetchInstagramOEmbed(ctx, shortcode)
	if err == nil && res != nil && len(res.Items) > 0 {
		return res, nil
	}

	return nil, fmt.Errorf("failed to extract Instagram media for code %s", shortcode)
}

func (c *Client) fetchInstagramEmbed(ctx context.Context, shortcode string) (*Result, error) {
	embedURL := fmt.Sprintf("https://www.instagram.com/p/%s/embed/captioned/", shortcode)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, embedURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)
	m := igEmbedJSONRegex.FindStringSubmatch(html)
	if len(m) < 2 {
		return nil, fmt.Errorf("embed JSON pattern not found")
	}

	var embedArr []json.RawMessage
	if err := json.Unmarshal([]byte("["+m[1]+"]"), &embedArr); err != nil {
		return nil, err
	}

	var contextJSON string
	for _, raw := range embedArr {
		var data igEmbedData
		if err := json.Unmarshal(raw, &data); err == nil && data.ContextJSON != "" {
			contextJSON = data.ContextJSON
			break
		}
	}

	if contextJSON == "" {
		return nil, fmt.Errorf("contextJSON missing from embed data")
	}

	var mediaRoot igShortcodeMedia
	if err := json.Unmarshal([]byte(contextJSON), &mediaRoot); err != nil {
		return nil, err
	}

	media := mediaRoot.ShortcodeMedia
	var items []MediaItem
	isPhoto := true

	if media.Sidecar != nil && len(media.Sidecar.Edges) > 0 {
		for i, edge := range media.Sidecar.Edges {
			node := edge.Node
			ext := "jpg"
			mediaType := "photo"
			targetURL := node.DisplayURL

			if node.IsVideo && node.VideoURL != "" {
				ext = "mp4"
				mediaType = "video"
				targetURL = node.VideoURL
				isPhoto = false
			}

			if targetURL != "" {
				items = append(items, MediaItem{
					URL:      targetURL,
					Type:     mediaType,
					Filename: fmt.Sprintf("instagram_%s_%d.%s", shortcode, i+1, ext),
					ThumbURL: node.DisplayURL,
				})
			}
		}
	} else if media.IsVideo && media.VideoURL != "" {
		isPhoto = false
		items = append(items, MediaItem{
			URL:      media.VideoURL,
			Type:     "video",
			Filename: fmt.Sprintf("instagram_%s.mp4", shortcode),
			ThumbURL: media.DisplayURL,
		})
	} else if media.DisplayURL != "" {
		items = append(items, MediaItem{
			URL:      media.DisplayURL,
			Type:     "photo",
			Filename: fmt.Sprintf("instagram_%s.jpg", shortcode),
		})
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("no items found in embed payload")
	}

	return &Result{
		Service: "instagram",
		ID:      shortcode,
		Items:   items,
		IsPhoto: isPhoto,
	}, nil
}

type igOEmbedResponse struct {
	Title        string `json:"title"`
	AuthorName   string `json:"author_name"`
	ThumbnailURL string `json:"thumbnail_url"`
	MediaID      string `json:"media_id"`
}

type igMobileMediaResponse struct {
	Items []struct {
		VideoVersions []struct {
			Width  int    `json:"width"`
			Height int    `json:"height"`
			URL    string `json:"url"`
		} `json:"video_versions"`
		ImageVersions2 struct {
			Candidates []struct {
				URL string `json:"url"`
			} `json:"candidates"`
		} `json:"image_versions2"`
		CarouselMedia []struct {
			VideoVersions []struct {
				Width  int    `json:"width"`
				Height int    `json:"height"`
				URL    string `json:"url"`
			} `json:"video_versions"`
			ImageVersions2 struct {
				Candidates []struct {
					URL string `json:"url"`
				} `json:"candidates"`
			} `json:"image_versions2"`
		} `json:"carousel_media"`
	} `json:"items"`
}

func (c *Client) fetchInstagramOEmbed(ctx context.Context, shortcode string) (*Result, error) {
	oembedURL := fmt.Sprintf("https://i.instagram.com/api/v1/oembed/?url=https://www.instagram.com/p/%s/", shortcode)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, oembedURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Instagram 275.0.0.27.98 Android (33/13; 280dpi; 720x1423; Xiaomi; Redmi 7; onclite; qcom; en_US; 458229237)")
	req.Header.Set("X-IG-App-ID", "936619743392459")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oembed HTTP status %d", resp.StatusCode)
	}

	var oembed igOEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&oembed); err != nil {
		return nil, err
	}

	if oembed.MediaID != "" {
		infoURL := fmt.Sprintf("https://i.instagram.com/api/v1/media/%s/info/", oembed.MediaID)
		infoReq, errInfo := http.NewRequestWithContext(ctx, http.MethodGet, infoURL, nil)
		if errInfo == nil {
			infoReq.Header.Set("User-Agent", "Instagram 275.0.0.27.98 Android (33/13; 280dpi; 720x1423; Xiaomi; Redmi 7; onclite; qcom; en_US; 458229237)")
			infoReq.Header.Set("X-IG-App-ID", "936619743392459")

			infoResp, errDo := c.HTTPClient.Do(infoReq)
			if errDo == nil && infoResp.StatusCode == http.StatusOK {
				defer infoResp.Body.Close()
				var mobileResp igMobileMediaResponse
				if json.NewDecoder(infoResp.Body).Decode(&mobileResp) == nil && len(mobileResp.Items) > 0 {
					item := mobileResp.Items[0]
					var items []MediaItem
					isPhoto := true

					if len(item.CarouselMedia) > 0 {
						for i, cm := range item.CarouselMedia {
							if len(cm.VideoVersions) > 0 {
								best := cm.VideoVersions[0]
								for _, v := range cm.VideoVersions {
									if v.Width*v.Height > best.Width*best.Height {
										best = v
									}
								}
								isPhoto = false
								items = append(items, MediaItem{
									URL:      best.URL,
									Type:     "video",
									Filename: fmt.Sprintf("instagram_%s_%d.mp4", shortcode, i+1),
								})
							} else if len(cm.ImageVersions2.Candidates) > 0 {
								items = append(items, MediaItem{
									URL:      cm.ImageVersions2.Candidates[0].URL,
									Type:     "photo",
									Filename: fmt.Sprintf("instagram_%s_%d.jpg", shortcode, i+1),
								})
							}
						}
					} else if len(item.VideoVersions) > 0 {
						best := item.VideoVersions[0]
						for _, v := range item.VideoVersions {
							if v.Width*v.Height > best.Width*best.Height {
								best = v
							}
						}
						isPhoto = false
						items = append(items, MediaItem{
							URL:      best.URL,
							Type:     "video",
							Filename: fmt.Sprintf("instagram_%s.mp4", shortcode),
						})
					} else if len(item.ImageVersions2.Candidates) > 0 {
						items = append(items, MediaItem{
							URL:      item.ImageVersions2.Candidates[0].URL,
							Type:     "photo",
							Filename: fmt.Sprintf("instagram_%s.jpg", shortcode),
						})
					}

					if len(items) > 0 {
						return &Result{
							Service: "instagram",
							ID:      shortcode,
							Title:   oembed.Title,
							Author:  oembed.AuthorName,
							Items:   items,
							IsPhoto: isPhoto,
						}, nil
					}
				}
			}
		}
	}

	if oembed.ThumbnailURL != "" {
		return &Result{
			Service: "instagram",
			ID:      shortcode,
			Title:   oembed.Title,
			Author:  oembed.AuthorName,
			IsPhoto: true,
			Items: []MediaItem{
				{
					URL:      oembed.ThumbnailURL,
					Type:     "photo",
					Filename: fmt.Sprintf("instagram_%s.jpg", shortcode),
				},
			},
		}, nil
	}

	return nil, fmt.Errorf("oembed fallback failed")
}

func extractInstagramShortcode(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	m := igShortcodeRegex.FindStringSubmatch(u.Path)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}
