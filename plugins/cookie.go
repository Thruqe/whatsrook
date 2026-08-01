// Cookie command – guides user to export browser cookies in Netscape format and save them per platform.
package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"whatsrook/store/sqlstore"
)

const (
	CookieSettingKeyPrefix = "platform_cookie_"
	EmberCookieEndpoint    = "https://embers-0kn7.onrender.com/cookies"
)

func init() {
	Register(&Command{
		Name:        "cookie",
		Aliases:     []string{"cookies"},
		Description: "Show tutorial and instructions on exporting platform cookies in Netscape format",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleCookieInstruction,
	})

	Register(&Command{
		Name:        "setcookie",
		Description: "Save exported Netscape cookie data for a platform (e.g. setcookie YOUTUBE <cookies>)",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleSetCookie,
	})
}

func handleCookieInstruction(ctx *Context) error {
	imgPath := filepath.Join("resources", "tutorials", "images", "cookies.png")

	imgBytes, err := os.ReadFile(imgPath)
	if err == nil && len(imgBytes) > 0 {
		caption := "Download & install the Cookie Editor browser extension to get your cookies:\nhttps://cookie-editor.com/#download\n\nExport your cookies in Netscape format by clicking 'Export' -> 'Export as Netscape' (or 'Copy as Netscape') as shown in the tutorial image."
		_ = ctx.ReplyWithImage(imgBytes, "image/png", caption)
	}

	prefix := ctx.GetPrefix()
	bodyText := fmt.Sprintf("1. Download & install Cookie Editor:\nhttps://cookie-editor.com/#download\n\n2. Open the platform site (e.g., YouTube, Twitter), open Cookie Editor, and copy/export your cookies in Netscape format.\n\n3. Paste your Netscape cookies after the %ssetcookie command along with the platform name (e.g. `%ssetcookie YOUTUBE <cookies>` or `%ssetcookie TWITTER <cookies>`).", prefix, prefix, prefix)
	return ctx.Reply(bodyText)
}

func normalizePlatformDomain(platform string) string {
	p := strings.ToLower(strings.TrimSpace(platform))
	switch p {
	case "youtube", "yt", "youtube.com":
		return "youtube.com"
	case "twitter", "x", "twitter.com", "x.com":
		return "x.com"
	case "instagram", "ig", "instagram.com":
		return "instagram.com"
	case "tiktok", "tiktok.com":
		return "tiktok.com"
	case "facebook", "fb", "facebook.com":
		return "facebook.com"
	case "threads", "threads.net":
		return "threads.net"
	default:
		if !strings.Contains(p, ".") {
			return p + ".com"
		}
		return p
	}
}

func handleSetCookie(ctx *Context) error {
	prefix := ctx.GetPrefix()
	rawArgs := strings.TrimSpace(ctx.RawArgs)
	var platformArg string
	var cookieData string

	if len(ctx.Args) > 0 {
		platformArg = ctx.Args[0]
		if len(ctx.Args) > 1 {
			// Extract everything after the first argument (platform)
			idx := strings.Index(rawArgs, platformArg)
			if idx != -1 {
				cookieData = strings.TrimSpace(rawArgs[idx+len(platformArg):])
			}
		}
	}

	if cookieData == "" {
		if quoted := ctx.GetQuotedMessage(); quoted != nil {
			if txt := extractTextFromProto(quoted); txt != "" {
				cookieData = strings.TrimSpace(txt)
			}
		}
	}

	if platformArg == "" || cookieData == "" {
		return ctx.Reply(fmt.Sprintf("Download Cookie Editor to get your Netscape cookies:\nhttps://cookie-editor.com/#download\n\nUsage: `%ssetcookie PLATFORM <netscape_cookie>`\nExample: `%ssetcookie YOUTUBE <cookies>` or `%ssetcookie TWITTER <cookies>`", prefix, prefix, prefix))
	}

	domain := normalizePlatformDomain(platformArg)

	if !IsValidNetscapeCookie(cookieData) {
		return ctx.Reply("Invalid cookie format. Cookies must be in valid Netscape format.\n\nDownload Cookie Editor to get your Netscape cookies:\nhttps://cookie-editor.com/#download")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Failed to access storage.")
	}

	// Save to local SQL Store
	settingKey := CookieSettingKeyPrefix + domain
	if err := s.PutSetting(ctx.Ctx, settingKey, cookieData); err != nil {
		slog.Error("Failed to save cookie data locally", "domain", domain, "err", err)
		return ctx.Reply("Failed to save cookie data.")
	}

	// Post to Embers API /cookies
	if err := PostCookieToEmber(ctx.Ctx, domain, cookieData); err != nil {
		slog.Warn("Failed to post cookie to Embers API", "domain", domain, "err", err)
	}

	return ctx.Reply(fmt.Sprintf("Cookie saved successfully for platform %s (%s).", strings.ToUpper(platformArg), domain))
}

// IsValidNetscapeCookie checks if the provided string follows Netscape HTTP Cookie File format.
func IsValidNetscapeCookie(data string) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return false
	}

	lines := strings.Split(trimmed, "\n")
	validCookieLines := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) >= 7 {
			validCookieLines++
		}
	}

	return validCookieLines > 0
}

// PostCookieToEmber posts cookie data to the Embers API /cookies endpoint.
func PostCookieToEmber(ctx context.Context, domain string, cookieData string) error {
	reqBody, err := json.Marshal(map[string]string{
		"domain":  domain,
		"cookies": cookieData,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, EmberCookieEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("embers /cookies returned status %d", resp.StatusCode)
	}
	return nil
}

// GetYouTubeCookieFile retrieves valid Netscape cookie data for YouTube from storage.
func GetYouTubeCookieFile(ctx *Context) (string, func(), bool) {
	if ctx == nil || ctx.Client == nil || ctx.Client.Store == nil {
		return "", func() {}, false
	}
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return "", func() {}, false
	}

	cookieData, err := s.GetSetting(ctx.Ctx, CookieSettingKeyPrefix+"youtube.com")
	if err != nil || cookieData == "" {
		return "", func() {}, false
	}

	tmpFile, err := os.CreateTemp("", "yt_cookie_*.txt")
	if err != nil {
		slog.Error("Failed to create temp cookie file", "err", err)
		return "", func() {}, false
	}

	if _, err := tmpFile.WriteString(cookieData); err != nil {
		slog.Error("Failed to write temp cookie file", "err", err)
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", func() {}, false
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	cleanup := func() {
		os.Remove(tmpPath)
	}

	return tmpPath, cleanup, true
}
