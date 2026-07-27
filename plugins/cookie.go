// Cookie command – guides user to export browser cookies in Netscape format and save them for 10 minutes.
package commands

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"whatsrook/store/sqlstore"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

const (
	CookieSettingKey        = "youtube_cookie"
	CookieExpiresSettingKey = "youtube_cookie_expires"
	CookieTTL               = 10 * time.Minute
)

func init() {
	Register(&Command{
		Name:        "cookie",
		Aliases:     []string{"cookies"},
		Description: "Show tutorial and instructions on exporting YouTube cookies in Netscape format",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleCookieInstruction,
	})

	Register(&Command{
		Name:        "setcookie",
		Description: "Save exported Netscape cookie data for YouTube downloads (expires in 10 minutes)",
		Category:    "media",
		IsPublic:    true,
		Handler:     handleSetCookie,
	})
}

func handleCookieInstruction(ctx *Context) error {
	imgPath := filepath.Join("resources", "tutorials", "images", "cookies.png")

	imgBytes, err := os.ReadFile(imgPath)
	if err == nil && len(imgBytes) > 0 {
		caption := "Download and install Get cookies.txt LOCALLY extension for your browser. Export your YouTube cookies as Netscape format by clicking Copy as Netscape as shown in the image above."
		_ = ctx.ReplyWithImage(imgBytes, "image/png", caption)
	}

	prefix := ctx.GetPrefix()
	btnID := prefix + "setcookie"
	btnJSON := fmt.Sprintf(`{"display_text":"Set Cookie","id":%q}`, btnID)
	msgVersion := int32(1)

	bodyText := "Bot is awaiting you. Click the button below or paste your Netscape cookies after .setcookie command."

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				InteractiveMessage: &waE2E.InteractiveMessage{
					Body: &waE2E.InteractiveMessage_Body{
						Text: &bodyText,
					},
					Footer: &waE2E.InteractiveMessage_Footer{
						Text: new("Powered by WhatsRook"),
					},
					InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
						NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
							Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
								{
									Name:             new("quick_reply"),
									ButtonParamsJSON: &btnJSON,
								},
							},
							MessageVersion: &msgVersion,
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
				Tag: "interactive",
				Attrs: waBinary.Attrs{
					"type": "native_flow",
					"v":    "1",
				},
				Content: []waBinary.Node{
					{
						Tag: "native_flow",
						Attrs: waBinary.Attrs{
							"v":    "9",
							"name": "mixed",
						},
					},
				},
			},
		},
	}

	extra := whatsmeow.SendRequestExtra{
		AdditionalNodes: &[]waBinary.Node{bizNode},
	}

	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, msg, extra)
	return err
}

func handleSetCookie(ctx *Context) error {
	cookieData := strings.TrimSpace(ctx.RawArgs)

	if cookieData == "" {
		if quoted := ctx.GetQuotedMessage(); quoted != nil {
			if txt := extractTextFromProto(quoted); txt != "" {
				cookieData = strings.TrimSpace(txt)
			}
		}
	}

	if cookieData == "" {
		prefix := ctx.GetPrefix()
		btnID := prefix + "cookie"
		btnJSON := fmt.Sprintf(`{"display_text":"View Tutorial","id":%q}`, btnID)
		msgVersion := int32(1)

		bodyText := "Bot is awaiting you, paste your cookies and nothing else after .setcookie command."

		msg := &waE2E.Message{
			DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
				Message: &waE2E.Message{
					InteractiveMessage: &waE2E.InteractiveMessage{
						Body: &waE2E.InteractiveMessage_Body{
							Text: &bodyText,
						},
						Footer: &waE2E.InteractiveMessage_Footer{
							Text: new("Powered by WhatsRook"),
						},
						InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
							NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
								Buttons: []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
									{
										Name:             new("quick_reply"),
										ButtonParamsJSON: &btnJSON,
									},
								},
								MessageVersion: &msgVersion,
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
					Tag: "interactive",
					Attrs: waBinary.Attrs{
						"type": "native_flow",
						"v":    "1",
					},
					Content: []waBinary.Node{
						{
							Tag: "native_flow",
							Attrs: waBinary.Attrs{
								"v":    "9",
								"name": "mixed",
							},
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

	if !IsValidNetscapeCookie(cookieData) {
		return ctx.Reply("Invalid Netscape cookie format. Please make sure you copy cookies in Netscape format.")
	}

	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Failed to access storage.")
	}

	expiresAt := time.Now().Add(CookieTTL).Unix()
	if err := s.PutSetting(ctx.Ctx, CookieSettingKey, cookieData); err != nil {
		slog.Error("Failed to save cookie data", "err", err)
		return ctx.Reply("Failed to save cookie data.")
	}

	if err := s.PutSetting(ctx.Ctx, CookieExpiresSettingKey, fmt.Sprintf("%d", expiresAt)); err != nil {
		slog.Error("Failed to save cookie expiration", "err", err)
	}

	return ctx.Reply("Cookie saved successfully. It will expire in 10 minutes.")
}

// IsValidNetscapeCookie checks if the provided string follows Netscape HTTP Cookie File format.
func IsValidNetscapeCookie(data string) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return false
	}

	lines := strings.Split(trimmed, "\n")
	hasNetscapeHeader := false
	validCookieLines := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "netscape") || strings.Contains(lower, "cookie") {
				hasNetscapeHeader = true
			}
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) >= 7 {
			domain := strings.TrimSpace(parts[0])
			sub := strings.ToUpper(strings.TrimSpace(parts[1]))
			sec := strings.ToUpper(strings.TrimSpace(parts[3]))

			if len(domain) > 0 && (sub == "TRUE" || sub == "FALSE") && (sec == "TRUE" || sec == "FALSE") {
				validCookieLines++
			} else if len(domain) > 0 && strings.Contains(domain, ".") {
				validCookieLines++
			}
		}
	}

	return validCookieLines > 0 || (hasNetscapeHeader && len(lines) > 1)
}

// GetYouTubeCookieFile retrieves valid Netscape cookie data from storage.
// If valid and unexpired (< 10 mins), writes it to a temporary file and returns the path, cleanup function, and true.
// If missing or expired (> 10 mins), clears the stored setting and returns false.
func GetYouTubeCookieFile(ctx *Context) (string, func(), bool) {
	if ctx == nil || ctx.Client == nil || ctx.Client.Store == nil {
		return "", func() {}, false
	}
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return "", func() {}, false
	}

	expiresStr, err := s.GetSetting(ctx.Ctx, CookieExpiresSettingKey)
	if err != nil || expiresStr == "" {
		return "", func() {}, false
	}

	var expiresAt int64
	if _, err := fmt.Sscanf(expiresStr, "%d", &expiresAt); err != nil {
		return "", func() {}, false
	}

	now := time.Now().Unix()
	if now >= expiresAt {
		slog.Debug("YouTube cookie expired, clearing", "now", now, "expiresAt", expiresAt)
		_ = s.DeleteSetting(ctx.Ctx, CookieSettingKey)
		_ = s.DeleteSetting(ctx.Ctx, CookieExpiresSettingKey)
		return "", func() {}, false
	}

	cookieData, err := s.GetSetting(ctx.Ctx, CookieSettingKey)
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
