// Menu command – lists all available commands with descriptions.
package commands

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"whatsrook/store/sqlstore"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

var (
	menuThumbPromptsMu      sync.RWMutex
	pendingMenuThumbPrompts = make(map[string]bool) // key: "chatJID:senderJID" -> active
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

	key := fmt.Sprintf("%s:%s", evt.Info.Chat.String(), evt.Info.Sender.String())
	menuThumbPromptsMu.RLock()
	active := pendingMenuThumbPrompts[key]
	menuThumbPromptsMu.RUnlock()

	if !active {
		return false
	}

	imgMsg := evt.Message.GetImageMessage()
	vidMsg := evt.Message.GetVideoMessage()

	// Check if quoted message contains image or video
	if imgMsg == nil && vidMsg == nil && evt.Message.ExtendedTextMessage != nil && evt.Message.ExtendedTextMessage.ContextInfo != nil {
		quoted := evt.Message.ExtendedTextMessage.ContextInfo.QuotedMessage
		if quoted != nil {
			imgMsg = quoted.GetImageMessage()
			vidMsg = quoted.GetVideoMessage()
		}
	}

	if imgMsg == nil && vidMsg == nil {
		return false
	}

	// Delete pending prompt
	menuThumbPromptsMu.Lock()
	delete(pendingMenuThumbPrompts, key)
	menuThumbPromptsMu.Unlock()

	fakeCtx := &Context{
		Ctx:    ctx,
		Client: client,
		Chat:   evt.Info.Chat,
		Sender: evt.Info.Sender,
		Evt:    evt,
	}

	loader := fakeCtx.StartLoader("Processing custom thumbnail...")
	defer loader.Delete()

	var data []byte
	var err error
	isVideo := false

	if vidMsg != nil {
		data, err = client.Download(ctx, vidMsg)
		isVideo = true
	} else if imgMsg != nil {
		data, err = client.Download(ctx, imgMsg)
	}

	if err != nil || len(data) == 0 {
		_ = fakeCtx.Reply("Failed to download media for menu thumbnail.")
		return true
	}

	_ = os.MkdirAll("tmp/songs", 0755)
	targetPath := "tmp/songs/custom_menu_thumbnail.mp4"

	if isVideo {
		if err := os.WriteFile(targetPath, data, 0644); err != nil {
			_ = fakeCtx.Reply("Failed to save video thumbnail: " + err.Error())
			return true
		}
	} else {
		tmpImg := fmt.Sprintf("/tmp/thumb_%d.jpg", time.Now().UnixNano())
		_ = os.WriteFile(tmpImg, data, 0644)
		defer os.Remove(tmpImg)

		cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-loop", "1", "-i", tmpImg, "-c:v", "libx264", "-t", "2", "-pix_fmt", "yuv420p", "-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2", targetPath)
		if err := cmd.Run(); err != nil {
			targetPath = "tmp/songs/custom_menu_thumbnail.jpg"
			_ = os.WriteFile(targetPath, data, 0644)
		}
	}

	if s, ok := client.Store.Identities.(*sqlstore.SQLStore); ok {
		_ = s.PutSetting(ctx, "menu_thumbnail_path", targetPath)
	}

	_ = fakeCtx.Reply("Bot menu thumbnail updated successfully! Type .menu to view your custom thumbnail.")
	return true
}

func handleMenu(ctx *Context) error {
	args := strings.Fields(ctx.RawArgs)
	if len(args) > 0 {
		sub := strings.ToLower(args[0])
		if sub == "customize" || sub == "custom" {
			key := fmt.Sprintf("%s:%s", ctx.Chat.String(), ctx.Sender.String())
			menuThumbPromptsMu.Lock()
			pendingMenuThumbPrompts[key] = true
			menuThumbPromptsMu.Unlock()
			return ctx.Reply("Upload or reply with an image (.jpg/.png) or video (.mp4) to set it as the custom bot menu thumbnail.\n\nTo restore default: " + ctx.GetPrefix() + "menu reset")
		}
		if sub == "reset" {
			key := fmt.Sprintf("%s:%s", ctx.Chat.String(), ctx.Sender.String())
			menuThumbPromptsMu.Lock()
			delete(pendingMenuThumbPrompts, key)
			menuThumbPromptsMu.Unlock()

			if s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore); ok {
				_ = s.PutSetting(ctx.Ctx, "menu_thumbnail_path", "")
			}
			_ = os.Remove("tmp/songs/custom_menu_thumbnail.mp4")
			_ = os.Remove("tmp/songs/custom_menu_thumbnail.jpg")
			return ctx.Reply("Bot menu thumbnail reset to default (whatsrook.mp4).")
		}
	}

	type entry struct{ name, desc string }
	categoryOrder := []string{}
	categories := map[string][]entry{}
	seenCat := map[string]bool{}

	for _, cmd := range Visible() {
		cat := cmd.Category
		if cat == "" {
			cat = "misc"
		}
		if !seenCat[cat] {
			seenCat[cat] = true
			categoryOrder = append(categoryOrder, cat)
		}
		categories[cat] = append(categories[cat], entry{name: cmd.Name, desc: cmd.Description})
	}

	uptime := menuRuntime(time.Since(startTime).Seconds())
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	usedRAM := ms.Alloc
	platform := runtime.GOOS
	total := len(Visible())

	user := ctx.Evt.Info.PushName
	if user == "" {
		user = ctx.Sender.User
	}

	botMode := "public"
	buildChannel := "Stable"
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if ok {
		if rawMode, err := s.GetSetting(ctx.Ctx, "mode"); err == nil && rawMode != "" {
			botMode = rawMode
		}
		if rawCh, err := s.GetSetting(ctx.Ctx, "update_channel"); err == nil && rawCh != "" {
			if strings.EqualFold(rawCh, "beta") {
				buildChannel = "Beta"
			}
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "╭━━━〔 %s 〕━━━\n", toFancy(ctx.GetBotName()))
	fmt.Fprintf(&sb, "│╭──────────────\n")
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("User    : %s", user)))
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("Os: %s", platform)))
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("Usage   : %s", formatBytes(usedRAM))))
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("Plugins : %d", total)))
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("Build   : %s", buildChannel)))
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("Mode    : %s", botMode)))
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("Uptime : %s", uptime)))
	fmt.Fprintf(&sb, "││ %s\n", toFancy(fmt.Sprintf("Version : %s", "4.0.0")))
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
		videoPath := "tmp/songs/custom_menu_thumbnail.mp4"
		if ok {
			if custom, err := s.GetSetting(ctx.Ctx, "menu_thumbnail_path"); err == nil && custom != "" {
				videoPath = custom
			}
		}
		if _, err := os.Stat(videoPath); err != nil {
			videoPath = "resources/songs/whatsrook.mp4"
			if _, err := os.Stat(videoPath); err != nil {
				videoPath = "resources/songs/intro.mp4"
			}
		}

		if videoData, err := os.ReadFile(videoPath); err == nil && len(videoData) > 0 {
			mType := "video/mp4"
			if strings.HasSuffix(videoPath, ".jpg") || strings.HasSuffix(videoPath, ".jpeg") {
				return ctx.ReplyWithImage(videoData, "image/jpeg", menuText)
			}
			return ctx.ReplyWithVideoGif(videoData, mType, menuText)
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
