// Cookie management plugin – manages YouTube Netscape cookies per session.
package plugins

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"whatsrook/wa-core/store/sqlstore"
)

func init() {
	Register(&Command{
		Name:        "cookie",
		Aliases:     []string{"setcookie", "ytcookie", "updatecookie"},
		Description: "Set or update Netscape YouTube cookies for the active session",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handleCookie,
	})
}

func handleCookie(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to bot owner and sudoers.")
	}

	p := ctx.GetPrefix()
	cookieContent := strings.TrimSpace(ctx.RawArgs)

	if cookieContent == "" {
		if quoted := ctx.GetQuotedMessage(); quoted != nil {
			if ext := quoted.GetExtendedTextMessage(); ext != nil {
				cookieContent = strings.TrimSpace(ext.GetText())
			} else if conv := quoted.GetConversation(); conv != "" {
				cookieContent = strings.TrimSpace(conv)
			}
		}
	}

	if cookieContent == "" {
		return ctx.Reply(fmt.Sprintf("Please provide Netscape cookie text or reply to a cookie message.\n\nUsage: %scookie <netscape_cookie_text>", p))
	}

	authDir := GetSessionAuthDir(ctx.Client)
	_ = os.MkdirAll(authDir, 0755)

	cookieFile := filepath.Join(authDir, "cookies.txt")
	err := os.WriteFile(cookieFile, []byte(cookieContent), 0644)
	if err != nil {
		slog.Error("handleCookie: failed to write cookies.txt", "path", cookieFile, "err", err)
		return ctx.Reply(fmt.Sprintf("Failed to save cookies file: %v", err))
	}

	if s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore); ok {
		_ = s.PutSetting(ctx.Ctx, "yt_cookie", cookieContent)
	}

	slog.Info("handleCookie: YouTube cookies updated for session", "authDir", authDir)
	return ctx.Reply(fmt.Sprintf("✅ YouTube Netscape cookies updated successfully for current session!\n\nFile saved to: `%s`", cookieFile))
}

// SendYTCookieHelp sends the cookie update tutorial image and step-by-step instructions when YouTube bot detection occurs.
func SendYTCookieHelp(ctx *Context) error {
	p := ctx.GetPrefix()
	tutorialImgPath := "/home/thruqe/Documents/whatsrook/resources/tutorials/images/cookies.png"

	var sb strings.Builder
	fmt.Fprintf(&sb, "⚠️ *YOUTUBE BOT DETECTION ALERT*\n\n")
	fmt.Fprintf(&sb, "YouTube is requesting bot verification (\"Sign in to confirm you're not a bot\"). You need to update your session's YouTube cookies!\n\n")
	fmt.Fprintf(&sb, "📋 *How to export & update your YouTube cookies*:\n")
	fmt.Fprintf(&sb, "1. If on mobile, download *Firefox* browser. (On Desktop, use Chrome/Firefox).\n")
	fmt.Fprintf(&sb, "2. Install the *Cookie-Editor* extension:\nhttps://chromewebstore.google.com/detail/cookie-editor/hlkenndednhfkekhgcdicdfddnkalmdm?hl=en\n")
	fmt.Fprintf(&sb, "3. Open https://youtube.com in your browser.\n")
	fmt.Fprintf(&sb, "4. Open Cookie-Editor, click *Export* -> *Export as Netscape*.\n")
	fmt.Fprintf(&sb, "5. Paste the exported cookie text into WhatsApp and run:\n`%scookie <paste_cookie_text>`\n\n", p)
	fmt.Fprintf(&sb, "See tutorial image below 👇")

	helpText := sb.String()

	imgData, err := os.ReadFile(tutorialImgPath)
	if err == nil && len(imgData) > 0 {
		return ctx.ReplyWithImage(imgData, "image/png", helpText)
	}

	return ctx.Reply(helpText)
}
