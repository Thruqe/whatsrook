// News command – fetches latest news headlines for a country from AP News.
package commands

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func init() {
	Register(&Command{
		Name:        "news",
		Aliases:     []string{"apnews", "topnews", "headlinenews"},
		Description: "Fetch latest news headlines for a country from AP News",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleNews,
	})
}

type NewsArticle struct {
	Title       string
	Description string
	URL         string
	ImageURL    string
}

var (
	promoRegex       = regexp.MustCompile(`(?s)<div class="PagePromo"[^>]*>(.*?)</div>\s*</div>\s*</div>`)
	linkRegex        = regexp.MustCompile(`href="(https?://apnews\.com/article/[^"]+|/article/[^"]+)"`)
	titleRegex       = regexp.MustCompile(`(?s)<h3 class="PagePromo-title"[^>]*>.*?<span class="PagePromoContentIcons-text">(.*?)</span>`)
	altTitleRegex    = regexp.MustCompile(`(?s)<h3 class="PagePromo-title"[^>]*>.*?<a[^>]*>(.*?)</a>`)
	descRegex        = regexp.MustCompile(`(?s)<div class="PagePromo-description"[^>]*>.*?<span class="PagePromoContentIcons-text">(.*?)</span>`)
	imageSrcsetRegex = regexp.MustCompile(`srcset="(https://dims\.apnews\.com/dims4/[^" ]+|https://assets\.apnews\.com/[^" ]+)"`)
	imageSrcRegex    = regexp.MustCompile(`src="(https://dims\.apnews\.com/dims4/[^"]+|https://assets\.apnews\.com/[^"]+)"`)
	stripTagsRegex   = regexp.MustCompile(`<[^>]*>`)
)

func handleNews(ctx *Context) error {
	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return sendNewsCountryMenu(ctx)
	}

	country := strings.ToLower(strings.TrimSpace(strings.Join(ctx.Args, "-")))
	hubURL := fmt.Sprintf("https://apnews.com/hub/%s", country)

	req, err := http.NewRequestWithContext(ctx.Ctx, http.MethodGet, hubURL, nil)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to create request: %v", err))
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to fetch news for %q: %v", country, err))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ctx.Reply(fmt.Sprintf("No news topic hub found for %q. Usage:\n- %snews <country_name>", country, p))
	} else if resp.StatusCode != http.StatusOK {
		return ctx.Reply(fmt.Sprintf("Failed to fetch news (HTTP %d).", resp.StatusCode))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ctx.Reply("Failed to read news response.")
	}
	htmlContent := string(bodyBytes)

	articles := parseAPNewsHTML(htmlContent)
	if len(articles) == 0 {
		return ctx.Reply(fmt.Sprintf("No recent news articles found for %q.", country))
	}

	var firstImageURL string
	var sb strings.Builder
	fmt.Fprintf(&sb, "AP News - %s\n\n", titleCase(strings.ReplaceAll(country, "-", " ")))

	count := 0
	for _, art := range articles {
		if count >= 5 {
			break
		}
		count++
		if firstImageURL == "" && art.ImageURL != "" {
			firstImageURL = art.ImageURL
		}

		fmt.Fprintf(&sb, "%d. %s\n", count, art.Title)
		if art.Description != "" {
			fmt.Fprintf(&sb, "   %s\n", art.Description)
		}
		if art.URL != "" {
			fmt.Fprintf(&sb, "   %s\n", art.URL)
		}
		sb.WriteString("\n")
	}

	responseText := strings.TrimSpace(sb.String())

	// Download first article thumbnail if available
	if firstImageURL != "" {
		imgReq, err := http.NewRequestWithContext(ctx.Ctx, http.MethodGet, firstImageURL, nil)
		if err == nil {
			imgReq.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
			imgResp, err := client.Do(imgReq)
			if err == nil && imgResp.StatusCode == http.StatusOK {
				imgData, err := io.ReadAll(imgResp.Body)
				imgResp.Body.Close()
				if err == nil && len(imgData) > 0 {
					mimetype := http.DetectContentType(imgData)
					return ctx.ReplyWithImage(imgData, mimetype, responseText)
				}
			}
			if imgResp != nil && imgResp.Body != nil {
				imgResp.Body.Close()
			}
		}
	}

	return ctx.Reply(responseText)
}

func parseAPNewsHTML(htmlContent string) []NewsArticle {
	var articles []NewsArticle
	seenURLs := make(map[string]bool)

	matches := promoRegex.FindAllString(htmlContent, -1)
	for _, match := range matches {
		var art NewsArticle

		// Extract URL
		if linkMatch := linkRegex.FindStringSubmatch(match); len(linkMatch) > 1 {
			art.URL = linkMatch[1]
			if strings.HasPrefix(art.URL, "/") {
				art.URL = "https://apnews.com" + art.URL
			}
		}
		if art.URL == "" || seenURLs[art.URL] {
			continue
		}

		// Extract Title
		if titleMatch := titleRegex.FindStringSubmatch(match); len(titleMatch) > 1 {
			art.Title = cleanHTMLText(titleMatch[1])
		} else if altMatch := altTitleRegex.FindStringSubmatch(match); len(altMatch) > 1 {
			art.Title = cleanHTMLText(altMatch[1])
		}
		if art.Title == "" {
			continue
		}

		// Extract Description
		if descMatch := descRegex.FindStringSubmatch(match); len(descMatch) > 1 {
			art.Description = cleanHTMLText(descMatch[1])
		}

		// Extract Image URL
		if imgMatch := imageSrcsetRegex.FindStringSubmatch(match); len(imgMatch) > 1 {
			art.ImageURL = html.UnescapeString(imgMatch[1])
		} else if srcMatch := imageSrcRegex.FindStringSubmatch(match); len(srcMatch) > 1 {
			art.ImageURL = html.UnescapeString(srcMatch[1])
		}

		seenURLs[art.URL] = true
		articles = append(articles, art)
	}

	return articles
}

func cleanHTMLText(input string) string {
	cleaned := stripTagsRegex.ReplaceAllString(input, "")
	cleaned = html.UnescapeString(cleaned)
	return strings.TrimSpace(cleaned)
}

func sendNewsCountryMenu(ctx *Context) error {
	p := ctx.GetPrefix()
	bodyText := "AP News Country Selector\n\nPlease select a country below to view top news headlines, or type a country name:\n\nExamples:\n- " + p + "news nigeria\n- " + p + "news japan\n- " + p + "news united-states\n- " + p + "news kenya"

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: new(bodyText),
					FooterText:  new("WhatsRook AP News"),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons: []*waE2E.ButtonsMessage_Button{
						{
							ButtonID: new(p + "news nigeria"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("NIGERIA"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(p + "news united-states"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("USA"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new(p + "news japan"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("JAPAN"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
					},
				},
			},
		},
	}

	bizNode := waBinary.Node{
		Tag:   "biz",
		Attrs: waBinary.Attrs{},
		Content: []waBinary.Node{
			{
				Tag:   "interactive",
				Attrs: waBinary.Attrs{"type": "native_flow", "v": "1"},
				Content: []waBinary.Node{
					{
						Tag:   "native_flow",
						Attrs: waBinary.Attrs{"name": "quick_reply"},
					},
				},
			},
		},
	}

	extra := whatsmeow.SendRequestExtra{
		AdditionalNodes: &[]waBinary.Node{bizNode},
	}

	_, err := ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	return err
}
