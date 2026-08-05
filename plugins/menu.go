// Menu command – lists all available commands with descriptions.
package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"whatsrook/store/sqlstore"
	"whatsrook/updater"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

var (
	menuThumbPromptsMu      sync.RWMutex
	pendingMenuThumbPrompts = make(map[string]time.Time) // key: "chatJID:senderJID" -> timestamp
)

func init() {
	Register(&Command{
		Name:        "menu",
		Description: "Show all available commands grouped by category",
		Category:    "info",
		IsPublic:    true,
		Handler:     handleMenu,
	})
}

// HandlePendingMenuMediaReply checks if the sender has an active menu thumbnail prompt and converts/saves their uploaded media.
func HandlePendingMenuMediaReply(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	if evt == nil || evt.Message == nil {
		return false
	}

	key := fmt.Sprintf("%s:%s", evt.Info.Chat.ToNonAD().String(), evt.Info.Sender.ToNonAD().String())
	menuThumbPromptsMu.RLock()
	promptTime, active := pendingMenuThumbPrompts[key]
	menuThumbPromptsMu.RUnlock()

	if active && time.Since(promptTime) > 5*time.Minute {
		menuThumbPromptsMu.Lock()
		delete(pendingMenuThumbPrompts, key)
		menuThumbPromptsMu.Unlock()
		active = false
	}

	if !active {
		return false
	}

	text := utils.ExtractMessageText(evt)
	fakeCtx := &Context{
		Ctx:    ctx,
		Client: client,
		Chat:   evt.Info.Chat,
		Sender: evt.Info.Sender,
		Evt:    evt,
	}

	// Bypass prompt if user types a command
	var prefixes []string
	if client != nil {
		prefixes = activePrefixes(ctx, client)
	} else {
		prefixes = []string{DefaultPrefix}
	}
	if text != "" {
		for _, pref := range prefixes {
			if pref != "" && strings.HasPrefix(text, pref) {
				menuThumbPromptsMu.Lock()
				delete(pendingMenuThumbPrompts, key)
				menuThumbPromptsMu.Unlock()
				return false
			}
		}
	}

	downloadable, isVideo, mime := ExtractMediaFromEvent(evt)
	if downloadable == nil {
		return false
	}

	// Delete pending prompt
	menuThumbPromptsMu.Lock()
	delete(pendingMenuThumbPrompts, key)
	menuThumbPromptsMu.Unlock()

	slog.Info("HandlePendingMenuMediaReply: Downloading custom menu media", "chat", key, "mime", mime, "isVideo", isVideo)
	loader := fakeCtx.StartLoader("Processing custom thumbnail...")
	data, err := client.Download(ctx, downloadable)
	loader.Delete()

	if err != nil || len(data) == 0 {
		slog.Error("HandlePendingMenuMediaReply: Download failed", "chat", key, "err", err)
		_ = fakeCtx.Reply("Failed to download media for menu thumbnail.")
		return true
	}

	authDir := GetSessionAuthDir(client)
	targetPath, errProc := ProcessAndSaveThumbnail(ctx, authDir, data, isVideo)
	if errProc != nil {
		_ = fakeCtx.Reply(fmt.Sprintf("Failed to process menu thumbnail: %v", errProc))
		return true
	}

	if s, ok := client.Store.Identities.(*sqlstore.SQLStore); ok {
		_ = s.PutSetting(ctx, "menu_thumbnail_path", targetPath)
	}

	_ = fakeCtx.Reply(fmt.Sprintf("Bot menu thumbnail updated successfully! Type %smenu to view your custom thumbnail.", fakeCtx.GetPrefix()))
	return true
}

func handleMenu(ctx *Context) error {
	args := strings.Fields(ctx.RawArgs)
	if len(args) > 0 {
		sub := strings.ToLower(args[0])
		if sub == "reconfigure" || sub == "reconfig" || sub == "wizard" || sub == "setup" {
			return handleReconfigure(ctx)
		}
		if sub == "customize" || sub == "custom" {
			key := fmt.Sprintf("%s:%s", ctx.Chat.ToNonAD().String(), ctx.Sender.ToNonAD().String())
			menuThumbPromptsMu.Lock()
			pendingMenuThumbPrompts[key] = time.Now()
			menuThumbPromptsMu.Unlock()
			return ctx.Reply("Upload or reply with an image (.jpg/.png) or video (.mp4) to set it as the custom bot menu thumbnail.\n\nTo restore default: " + ctx.GetPrefix() + "menu reset")
		}
		if sub == "reset" {
			key := fmt.Sprintf("%s:%s", ctx.Chat.ToNonAD().String(), ctx.Sender.ToNonAD().String())
			menuThumbPromptsMu.Lock()
			delete(pendingMenuThumbPrompts, key)
			menuThumbPromptsMu.Unlock()

			authDir := GetSessionAuthDir(ctx.Client)
			if s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore); ok {
				_ = s.PutSetting(ctx.Ctx, "menu_thumbnail_path", "")
			}
			_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.mp4"))
			_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.jpg"))
			return ctx.Reply("Bot menu thumbnail reset to default (whatsrook.mp4).")
		}
	}

	type entry struct{ name, desc string }
	categoryOrder := []string{}
	categories := map[string][]entry{}
	seenCat := map[string]bool{}

	hiddenCmds := map[string]bool{
		"menu":     true,
		"netpause": true,
	}

	displayedCount := 0
	for _, cmd := range Visible() {
		if hiddenCmds[strings.ToLower(cmd.Name)] {
			continue
		}
		cat := cmd.Category
		if cat == "" {
			cat = "misc"
		}
		if !seenCat[cat] {
			seenCat[cat] = true
			categoryOrder = append(categoryOrder, cat)
		}
		categories[cat] = append(categories[cat], entry{name: cmd.Name, desc: cmd.Description})
		displayedCount++
	}

	uptime := menuRuntime(time.Since(startTime).Seconds())
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	usedRAM := ms.Alloc
	platform := runtime.GOOS

	user := ctx.Evt.Info.PushName
	if user == "" {
		user = ctx.Sender.User
	}

	botMode := "public"
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if ok {
		if rawMode, err := s.GetSetting(ctx.Ctx, "mode"); err == nil && rawMode != "" {
			botMode = rawMode
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "╭━━━〔 %s 〕━━━\n", toFancy(ctx.GetBotName()))
	fmt.Fprintf(&sb, "│╭──────────────\n")
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("User    : %s", user)))
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("Os      : %s", platform)))
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("Mem     : %s", formatBytes(usedRAM))))
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("Plugins : %d", displayedCount)))
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("Mode    : %s", botMode)))
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("Uptime  : %s", uptime)))
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("Version : %s", updater.GetAppVersion())))
	fmt.Fprintf(&sb, "│╰──────────────\n")
	fmt.Fprintf(&sb, "╰━━━━━━━━━━━━━━━\n")

	for _, cat := range categoryOrder {
		cmds := categories[cat]
		catLabel := "*〔 " + toFancy(strings.ToUpper(cat)) + " 〕*"

		fmt.Fprintf(&sb, "╭─────────────\n")
		fmt.Fprintf(&sb, "│ %s\n", catLabel)
		fmt.Fprintf(&sb, "╰┬────────────\n")
		fmt.Fprintf(&sb, "┌┤\n")

		for _, e := range cmds {
			fmt.Fprintf(&sb, "││◦ %s\n", toFancy(e.name))
		}

		fmt.Fprintf(&sb, "│╰────────────\n")
		fmt.Fprintf(&sb, "╰─────────────\n\n")
	}

	menuText := strings.TrimRight(sb.String(), "\n")

	// Auto-switch: toggle setting "menu_style" between "text" and "video"
	menuStyle := "video"
	if ok {
		currentStyle, _ := s.GetSetting(ctx.Ctx, "menu_style")
		if currentStyle == "video" {
			menuStyle = "text"
		} else {
			menuStyle = "video"
		}
		_ = s.PutSetting(ctx.Ctx, "menu_style", menuStyle)
	}

	if menuStyle == "video" {
		authDir := GetSessionAuthDir(ctx.Client)
		videoPath := filepath.Join(authDir, "custom_menu_thumbnail.mp4")
		if ok {
			if custom, err := s.GetSetting(ctx.Ctx, "menu_thumbnail_path"); err == nil && custom != "" {
				videoPath = custom
			}
		}
		if _, err := os.Stat(videoPath); err != nil {
			jpgPath := filepath.Join(authDir, "custom_menu_thumbnail.jpg")
			if _, errJpg := os.Stat(jpgPath); errJpg == nil {
				videoPath = jpgPath
			} else {
				videoPath = "resources/songs/whatsrook.mp4"
				if _, err := os.Stat(videoPath); err != nil {
					videoPath = "resources/songs/intro.mp4"
				}
			}
		}

		if videoData, err := os.ReadFile(videoPath); err == nil && len(videoData) > 0 {
			mType := "video/mp4"
			if strings.HasSuffix(videoPath, ".jpg") || strings.HasSuffix(videoPath, ".jpeg") {
				return ctx.ReplyWithImage(videoData, "image/jpeg", menuText)
			}
			errSend := ctx.ReplyWithVideoGif(videoData, mType, menuText)
			if errSend != nil {
				return sendText(ctx, menuText)
			}
			return nil
		}
	}

	return sendText(ctx, menuText)
}

// menuRuntime formats a duration in seconds as "Xd Xh Xm Xs".
func menuRuntime(seconds float64) string {
	secs := int(math.Floor(seconds))
	d := secs / (3600 * 24)
	h := (secs % (3600 * 24)) / 3600
	m := (secs % 3600) / 60
	s := secs % 60

	var parts []string
	if d > 0 {
		parts = append(parts, fmt.Sprintf("%dd", d))
	}
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	if s > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", s))
	}
	return strings.Join(parts, " ")
}

// formatBytes formats a byte count into a human-readable string (KB/MB/GB).
func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func toFancy(s string) string {
	return utils.ConvertFontStyle(s)
}
