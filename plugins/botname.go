// Bot Customization plugin – unified setup wizard and settings for Bot Name, Menu Thumbnail, Command Prefix, and Status Bio.
package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"whatsrook/meta"
	"whatsrook/store/sqlstore"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	BotNameSettingKey          = "bot_name"
	BotNamePromptDismissedKey  = "botname_prompt_dismissed"
	BotNameAwaitingInputPrefix = "botname_awaiting_input:"
	wizardSessionTTL           = 5 * time.Minute
)

type wizardSession struct {
	Step      string
	UpdatedAt time.Time
}

var (
	botWizardMu        sync.RWMutex
	pendingWizardState = make(map[string]wizardSession) // key: "chatJID:senderJID" -> wizardSession
)

func init() {
	Register(&Command{
		Name:        "setbot",
		Aliases:     []string{"bot", "botconfig", "botname", "setbotname", "setprefix", "botthumb"},
		Description: "Unified Bot Customization Wizard (Bot Name, Menu Thumbnail, Prefix, Bio)",
		Category:    "settings",
		IsPublic:    false, // Hidden from public .menu listing per user request
		Handler:     handleSetBot,
	})

	Register(&Command{
		Name:        "reconfigure",
		Aliases:     []string{"reconfig", "reconfiguration", "setupwizard", "reconfiguremenu"},
		Description: "Reconfigure bot settings and re-bring the setup wizard",
		Category:    "settings",
		IsPublic:    true,
		Handler:     handleReconfigure,
	})
}

func handleReconfigure(ctx *Context) error {
	key := fmt.Sprintf("%s:%s", ctx.Chat.ToNonAD().String(), ctx.Sender.ToNonAD().String())
	botWizardMu.Lock()
	pendingWizardState[key] = wizardSession{Step: "name", UpdatedAt: time.Now()}
	botWizardMu.Unlock()

	p := ctx.GetPrefix()
	bodyText := "Bot Customization Wizard (Step 1/4)\n\nPlease enter your desired bot display name (e.g. Jarvis, Fuzzy, Meow):"
	return ctx.Reply(fmt.Sprintf("%s\n\n(Tip: Type %sreconfigure anytime to restart this wizard)", bodyText, p))
}

// GetSessionAuthDir returns the directory path for the current session's auth files.
func GetSessionAuthDir(client *whatsmeow.Client) string {
	if client != nil && client.Store != nil {
		if s, ok := client.Store.Identities.(*sqlstore.SQLStore); ok && s.SessionDir != "" {
			return s.SessionDir
		}
		if client.Store.ID != nil && client.Store.ID.User != "" {
			return filepath.Join("auth", client.Store.ID.User)
		}
	}
	return filepath.Join("auth", "default")
}

// ProcessAndSaveThumbnail compresses/trims and saves uploaded image or video as the custom menu thumbnail inside the session auth folder.
func ProcessAndSaveThumbnail(ctx context.Context, authDir string, data []byte, isVideo bool) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty media data")
	}

	if authDir == "" {
		authDir = filepath.Join("auth", "default")
	}
	_ = os.MkdirAll(authDir, 0755)
	targetPath := filepath.Join(authDir, "custom_menu_thumbnail.mp4")

	if isVideo {
		tempInput := filepath.Join(authDir, fmt.Sprintf("input_%d.mp4", time.Now().UnixNano()))
		if err := os.WriteFile(tempInput, data, 0644); err != nil {
			return "", fmt.Errorf("failed to save temp video: %w", err)
		}
		defer os.Remove(tempInput)

		// ffmpeg: trim to 5s max, scale down to 480p max, compress H.264 CRF 28 @ 15fps, no audio
		cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-i", tempInput,
			"-t", "5",
			"-vf", "scale='min(480,iw)':-2,fps=15",
			"-c:v", "libx264", "-preset", "fast", "-crf", "28",
			"-an", "-pix_fmt", "yuv420p",
			targetPath)

		if err := cmd.Run(); err != nil {
			slog.Warn("ffmpeg video processing failed, checking raw video fallback", "err", err)
			if len(data) <= 10*1024*1024 {
				if errWrite := os.WriteFile(targetPath, data, 0644); errWrite != nil {
					return "", fmt.Errorf("failed to write raw video fallback: %w", errWrite)
				}
			} else {
				return "", fmt.Errorf("video file too large (>10MB) and ffmpeg processing failed: %w", err)
			}
		}
	} else {
		tempImg := filepath.Join(authDir, fmt.Sprintf("thumb_%d.jpg", time.Now().UnixNano()))
		if err := os.WriteFile(tempImg, data, 0644); err != nil {
			return "", fmt.Errorf("failed to save temp image: %w", err)
		}
		defer os.Remove(tempImg)

		cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-loop", "1", "-i", tempImg,
			"-c:v", "libx264", "-t", "2", "-pix_fmt", "yuv420p",
			"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2", targetPath)
		if err := cmd.Run(); err != nil {
			targetPath = filepath.Join(authDir, "custom_menu_thumbnail.jpg")
			if errWrite := os.WriteFile(targetPath, data, 0644); errWrite != nil {
				return "", fmt.Errorf("failed to save raw image fallback: %w", errWrite)
			}
		}
	}

	return targetPath, nil
}

// ExtractMediaFromEvent extracts downloadable image, video, or document media from a WhatsApp message event.
func ExtractMediaFromEvent(evt *events.Message) (whatsmeow.DownloadableMessage, bool, string) {
	if evt == nil || evt.Message == nil {
		return nil, false, ""
	}

	msg := evt.Message
	if msg.EphemeralMessage != nil && msg.EphemeralMessage.Message != nil {
		msg = msg.EphemeralMessage.Message
	}
	if msg.ViewOnceMessage != nil && msg.ViewOnceMessage.Message != nil {
		msg = msg.ViewOnceMessage.Message
	} else if msg.ViewOnceMessageV2 != nil && msg.ViewOnceMessageV2.Message != nil {
		msg = msg.ViewOnceMessageV2.Message
	} else if msg.ViewOnceMessageV2Extension != nil && msg.ViewOnceMessageV2Extension.Message != nil {
		msg = msg.ViewOnceMessageV2Extension.Message
	}

	if img := msg.GetImageMessage(); img != nil {
		return img, false, img.GetMimetype()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid, true, vid.GetMimetype()
	}
	if doc := msg.GetDocumentMessage(); doc != nil {
		mime := doc.GetMimetype()
		filename := strings.ToLower(doc.GetFileName())
		if strings.HasPrefix(mime, "video/") || strings.HasSuffix(filename, ".mp4") || strings.HasSuffix(filename, ".mkv") {
			return doc, true, mime
		}
		if strings.HasPrefix(mime, "image/") || strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".png") || strings.HasSuffix(filename, ".jpeg") {
			return doc, false, mime
		}
	}

	// Check quoted message
	if ext := msg.GetExtendedTextMessage(); ext != nil && ext.GetContextInfo() != nil && ext.GetContextInfo().QuotedMessage != nil {
		q := ext.GetContextInfo().QuotedMessage
		if q.EphemeralMessage != nil && q.EphemeralMessage.Message != nil {
			q = q.EphemeralMessage.Message
		}
		if img := q.GetImageMessage(); img != nil {
			return img, false, img.GetMimetype()
		}
		if vid := q.GetVideoMessage(); vid != nil {
			return vid, true, vid.GetMimetype()
		}
		if doc := q.GetDocumentMessage(); doc != nil {
			mime := doc.GetMimetype()
			filename := strings.ToLower(doc.GetFileName())
			if strings.HasPrefix(mime, "video/") || strings.HasSuffix(filename, ".mp4") {
				return doc, true, mime
			}
			if strings.HasPrefix(mime, "image/") || strings.HasSuffix(filename, ".jpg") || strings.HasSuffix(filename, ".png") {
				return doc, false, mime
			}
		}
	}

	return nil, false, ""
}

// HandlePendingBotCustomizationReply intercepts messages for users currently in the setup wizard or prompt sessions.
func HandlePendingBotCustomizationReply(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	if evt == nil || evt.Message == nil {
		return false
	}

	senderUser := evt.Info.Sender.ToNonAD().User
	key := fmt.Sprintf("%s:%s", evt.Info.Chat.ToNonAD().String(), evt.Info.Sender.ToNonAD().String())

	botWizardMu.RLock()
	session, inWizard := pendingWizardState[key]
	botWizardMu.RUnlock()

	// Check session TTL (5 min inactivity timeout)
	if inWizard && time.Since(session.UpdatedAt) > wizardSessionTTL {
		botWizardMu.Lock()
		delete(pendingWizardState, key)
		botWizardMu.Unlock()
		inWizard = false
	}

	var s *sqlstore.SQLStore
	var okStore bool
	if client != nil && client.Store != nil {
		s, okStore = client.Store.Identities.(*sqlstore.SQLStore)
	}
	text := utils.ExtractMessageText(evt)

	fakeCtx := &Context{
		Ctx:    ctx,
		Client: client,
		Chat:   evt.Info.Chat,
		Sender: evt.Info.Sender,
		Evt:    evt,
	}
	p := fakeCtx.GetPrefix()
	var prefixes []string
	if client != nil {
		prefixes = activePrefixes(ctx, client)
	} else {
		prefixes = []string{DefaultPrefix}
	}

	// If user sends a command (starts with command prefix), auto-exit wizard and allow command to dispatch!
	if inWizard && text != "" {
		for _, pref := range prefixes {
			if pref != "" && strings.HasPrefix(text, pref) {
				botWizardMu.Lock()
				delete(pendingWizardState, key)
				botWizardMu.Unlock()
				return false
			}
		}
	}

	if !inWizard && okStore {
		rawPrompt, _ := s.GetSetting(ctx, BotNameAwaitingInputPrefix+senderUser)
		if rawPrompt == "true" && text != "" && !strings.HasPrefix(text, p) {
			session = wizardSession{Step: "name", UpdatedAt: time.Now()}
			inWizard = true
		}
	}

	if !inWizard {
		return false
	}

	slog.Info("Wizard handling step", "chat", key, "step", session.Step, "text", text)

	switch session.Step {
	case "name":
		if text == "" {
			return false
		}
		newName := strings.TrimSpace(text)
		if okStore {
			_ = s.PutSetting(ctx, BotNameSettingKey, newName)
			_ = s.PutSetting(ctx, BotNamePromptDismissedKey, "true")
			_ = s.PutSetting(ctx, BotNameAwaitingInputPrefix+senderUser, "")
		}
		meta.ClearInstructionCache()

		botWizardMu.Lock()
		pendingWizardState[key] = wizardSession{Step: "thumb", UpdatedAt: time.Now()}
		botWizardMu.Unlock()

		msg := fmt.Sprintf("Bot name set to %q.\n\nBot Customization Wizard (Step 2/4)\n\nPlease upload or reply with an image (.jpg/.png) or video (.mp4) to set as your bot menu thumbnail.", newName)
		_ = fakeCtx.Reply(msg)
		return true

	case "thumb":
		downloadable, isVideo, mime := ExtractMediaFromEvent(evt)
		slog.Info("Wizard Step 2/4 (thumb): Checking media payload", "chat", key, "mime", mime, "isVideo", isVideo, "foundMedia", downloadable != nil)

		if downloadable == nil {
			slog.Warn("Wizard Step 2/4 (thumb): No image/video/document media found in message", "chat", key)
			_ = fakeCtx.Reply("Please upload or reply with an image (.jpg/.png) or video (.mp4) for the bot thumbnail.")
			return true
		}

		slog.Info("Wizard Step 2/4 (thumb): Starting media download", "chat", key, "mime", mime)
		loader := fakeCtx.StartLoader("Processing custom thumbnail...")
		data, err := client.Download(ctx, downloadable)
		loader.Delete()

		if err != nil || len(data) == 0 {
			slog.Error("Wizard Step 2/4 (thumb): Media download failed", "chat", key, "err", err, "dataLen", len(data))
			_ = fakeCtx.Reply(fmt.Sprintf("Failed to download media for thumbnail (error: %v). Please try sending another file.", err))
			return true
		}

		slog.Info("Wizard Step 2/4 (thumb): Media downloaded successfully", "chat", key, "bytesLen", len(data), "isVideo", isVideo)
		authDir := GetSessionAuthDir(client)
		targetPath, errProc := ProcessAndSaveThumbnail(ctx, authDir, data, isVideo)
		if errProc != nil {
			slog.Error("Wizard Step 2/4 (thumb): Thumbnail processing failed", "chat", key, "err", errProc)
			_ = fakeCtx.Reply(fmt.Sprintf("Failed to process thumbnail: %v", errProc))
			return true
		}

		slog.Info("Wizard Step 2/4 (thumb): Thumbnail saved successfully", "chat", key, "targetPath", targetPath)

		if okStore {
			_ = s.PutSetting(ctx, "menu_thumbnail_path", targetPath)
		}

		botWizardMu.Lock()
		pendingWizardState[key] = wizardSession{Step: "prefix", UpdatedAt: time.Now()}
		botWizardMu.Unlock()

		bodyText := "Bot menu thumbnail updated successfully.\n\nBot Customization Wizard (Step 3/4)\n\nPlease send the symbol or prefix you want to use (e.g. ., !, / or 'none').\n\nOr click Skip below to keep current prefix."
		buttons := []struct{ ID, Text string }{
			{ID: p + "setbot prompt_prefix", Text: "Set Prefix"},
			{ID: p + "setbot skip 3", Text: "Skip"},
		}
		_ = sendInteractiveButtons(fakeCtx, bodyText, fmt.Sprintf("%s Setup", fakeCtx.GetBotName()), buttons)
		return true

	case "prefix":
		if text == "" {
			return false
		}
		newPrefix := strings.TrimSpace(text)
		if strings.EqualFold(newPrefix, "none") || strings.EqualFold(newPrefix, "empty") {
			newPrefix = "empty"
		}
		if okStore {
			_ = s.PutSetting(ctx, PrefixSettingKey, newPrefix)
		}

		botWizardMu.Lock()
		pendingWizardState[key] = wizardSession{Step: "bio", UpdatedAt: time.Now()}
		botWizardMu.Unlock()

		bodyText := fmt.Sprintf("Prefix set to %q.\n\nBot Customization Wizard (Step 4/4)\n\nPlease send the text for your bot's WhatsApp status bio.\n\nOr click Skip to finish wizard.", newPrefix)
		buttons := []struct{ ID, Text string }{
			{ID: p + "setbot prompt_bio", Text: "Set Bio"},
			{ID: p + "setbot skip 4", Text: "Skip"},
		}
		_ = sendInteractiveButtons(fakeCtx, bodyText, fmt.Sprintf("%s Setup", fakeCtx.GetBotName()), buttons)
		return true

	case "bio":
		if text == "" {
			return false
		}
		newBio := strings.TrimSpace(text)
		_ = client.SetStatusMessage(ctx, types.SetStatusInput{Text: &newBio})

		botWizardMu.Lock()
		delete(pendingWizardState, key)
		botWizardMu.Unlock()

		_ = sendWizardSummaryCard(fakeCtx)
		return true
	}

	return false
}

func handleSetBot(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	p := ctx.GetPrefix()
	senderUser := ctx.Sender.ToNonAD().User
	key := fmt.Sprintf("%s:%s", ctx.Chat.ToNonAD().String(), ctx.Sender.ToNonAD().String())
	args := strings.Fields(ctx.RawArgs)

	if len(args) > 0 {
		sub := strings.ToLower(args[0])

		switch sub {
		case "wizard", "setup", "reconfigure", "reconfig":
			botWizardMu.Lock()
			pendingWizardState[key] = wizardSession{Step: "name", UpdatedAt: time.Now()}
			botWizardMu.Unlock()
			return ctx.Reply("Bot Customization Wizard (Step 1/4)\n\nPlease enter your desired bot display name (e.g. Jarvis, Fuzzy, Meow):")

		case "prompt_name", "name_prompt":
			botWizardMu.Lock()
			pendingWizardState[key] = wizardSession{Step: "name", UpdatedAt: time.Now()}
			botWizardMu.Unlock()
			return ctx.Reply("Please type your desired bot display name (e.g. Jarvis, Meow, Fuzzy):")

		case "prompt_thumb", "thumb_prompt":
			botWizardMu.Lock()
			pendingWizardState[key] = wizardSession{Step: "thumb", UpdatedAt: time.Now()}
			botWizardMu.Unlock()
			return ctx.Reply("Please upload or reply with an image (.jpg/.png) or video (.mp4) to set as your bot menu thumbnail.")

		case "prompt_prefix", "prefix_prompt":
			botWizardMu.Lock()
			pendingWizardState[key] = wizardSession{Step: "prefix", UpdatedAt: time.Now()}
			botWizardMu.Unlock()
			return ctx.Reply("Please send the command prefix symbol or word you want to use (e.g. ., !, / or 'none'):")

		case "prompt_bio", "bio_prompt":
			botWizardMu.Lock()
			pendingWizardState[key] = wizardSession{Step: "bio", UpdatedAt: time.Now()}
			botWizardMu.Unlock()
			return ctx.Reply("Please send the text for your bot's WhatsApp status bio:")

		case "skip":
			stepNum := 0
			if len(args) > 1 {
				stepNum, _ = strconv.Atoi(args[1])
			}

			if stepNum == 3 {
				botWizardMu.Lock()
				pendingWizardState[key] = wizardSession{Step: "bio", UpdatedAt: time.Now()}
				botWizardMu.Unlock()

				bodyText := "Bot Customization Wizard (Step 4/4)\n\nPlease send the text for your bot's WhatsApp status bio.\n\nOr click Skip to finish."
				buttons := []struct{ ID, Text string }{
					{ID: p + "setbot prompt_bio", Text: "Set Bio"},
					{ID: p + "setbot skip 4", Text: "Skip"},
				}
				return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("%s Setup", ctx.GetBotName()), buttons)
			}

			if stepNum == 4 || stepNum == 0 {
				botWizardMu.Lock()
				delete(pendingWizardState, key)
				botWizardMu.Unlock()
				return sendWizardSummaryCard(ctx)
			}

		case "page":
			pageNum := 1
			if len(args) > 1 {
				pageNum, _ = strconv.Atoi(args[1])
			}
			return sendSetBotPage(ctx, pageNum)

		case "name", "setname":
			if len(args) < 2 {
				return ctx.Reply(fmt.Sprintf("Usage: %sbotname <New Name>", p))
			}
			newName := strings.Join(args[1:], " ")
			_ = s.PutSetting(ctx.Ctx, BotNameSettingKey, newName)
			_ = s.PutSetting(ctx.Ctx, BotNamePromptDismissedKey, "true")
			_ = s.PutSetting(ctx.Ctx, BotNameAwaitingInputPrefix+senderUser, "")
			meta.ClearInstructionCache()
			return ctx.Reply(fmt.Sprintf("Bot name successfully updated to: %q!", newName))

		case "prefix", "setprefix":
			if len(args) < 2 {
				return ctx.Reply(fmt.Sprintf("Usage: %ssetprefix <symbol>", p))
			}
			newPrefix := args[1]
			if strings.EqualFold(newPrefix, "none") || strings.EqualFold(newPrefix, "empty") {
				newPrefix = "empty"
			}
			_ = s.PutSetting(ctx.Ctx, PrefixSettingKey, newPrefix)
			return ctx.Reply(fmt.Sprintf("Command prefix updated to: %q!", newPrefix))

		case "bio", "setbio":
			if len(args) < 2 {
				return ctx.Reply(fmt.Sprintf("Usage: %sbio <text>", p))
			}
			newBio := strings.Join(args[1:], " ")
			if err := ctx.Client.SetStatusMessage(ctx.Ctx, types.SetStatusInput{Text: &newBio}); err != nil {
				return ctx.Reply("Failed to update status bio: " + err.Error())
			}
			return ctx.Reply("Bot status bio updated successfully!")

		case "reset":
			authDir := GetSessionAuthDir(ctx.Client)
			_ = s.PutSetting(ctx.Ctx, BotNameSettingKey, "")
			_ = s.PutSetting(ctx.Ctx, BotNamePromptDismissedKey, "")
			_ = s.PutSetting(ctx.Ctx, PrefixSettingKey, "")
			_ = s.PutSetting(ctx.Ctx, "menu_thumbnail_path", "")
			_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.mp4"))
			_ = os.Remove(filepath.Join(authDir, "custom_menu_thumbnail.jpg"))
			meta.ClearInstructionCache()
			return ctx.Reply("All bot settings (Name, Thumbnail, Prefix) reset to default values.")

		case "setup_customize":
			botWizardMu.Lock()
			pendingWizardState[key] = wizardSession{Step: "name", UpdatedAt: time.Now()}
			botWizardMu.Unlock()
			return ctx.Reply("Bot Customization Wizard (Step 1/4)\n\nPlease enter your desired bot display name (e.g. Jarvis, Fuzzy, Meow):")

		case "setup_continue":
			_ = s.PutSetting(ctx.Ctx, BotNamePromptDismissedKey, "true")
			_ = s.PutSetting(ctx.Ctx, BotNameAwaitingInputPrefix+senderUser, "")
			bodyText := "BOT NAME CUSTOMIZATION RECOMMENDED: Keeping default name WhatsRook is fine. You can start the customization wizard anytime using " + p + "reconfigure:"
			buttons := []struct{ ID, Text string }{
				{ID: p + "reconfigure", Text: "Start Wizard"},
				{ID: p + "setbot setup_ignore", Text: "Keep Default"},
			}
			return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("Powered by %s", ctx.GetBotName()), buttons)

		case "setup_ignore":
			_ = s.PutSetting(ctx.Ctx, BotNamePromptDismissedKey, "true")
			_ = s.PutSetting(ctx.Ctx, BotNameAwaitingInputPrefix+senderUser, "")
			return ctx.Reply(fmt.Sprintf("Kept default bot name. Change anytime using %sreconfigure or %ssetbot", p, p))
		}
	}

	return sendSetBotPage(ctx, 1)
}

func sendSetBotPage(ctx *Context, pageNum int) error {
	p := ctx.GetPrefix()
	botName := ctx.GetBotName()
	curPrefix := p
	if curPrefix == "" {
		curPrefix = "(none)"
	}

	thumbStatus := "Default (whatsrook.mp4)"
	if s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore); ok {
		if custom, err := s.GetSetting(ctx.Ctx, "menu_thumbnail_path"); err == nil && custom != "" {
			if _, errStat := os.Stat(custom); errStat == nil {
				thumbStatus = "Custom Thumbnail"
			}
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "╭━━━〔 BOT CUSTOMIZATION 〕━━━\n")
	fmt.Fprintf(&sb, "│ Name      : %s\n", botName)
	fmt.Fprintf(&sb, "│ Thumbnail : %s\n", thumbStatus)
	fmt.Fprintf(&sb, "│ Prefix    : %s\n", curPrefix)
	fmt.Fprintf(&sb, "╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	var buttons []struct{ ID, Text string }

	switch pageNum {
	case 1:
		buttons = []struct{ ID, Text string }{
			{ID: p + "reconfigure", Text: "Wizard"},
			{ID: p + "setbot prompt_name", Text: "Bot Name"},
			{ID: p + "setbot page 2", Text: "Next ▶️"},
		}
	case 2:
		buttons = []struct{ ID, Text string }{
			{ID: p + "setbot prompt_thumb", Text: "Thumbnail"},
			{ID: p + "setbot prompt_prefix", Text: "Prefix"},
			{ID: p + "setbot page 3", Text: "Next ▶️"},
		}
	default:
		buttons = []struct{ ID, Text string }{
			{ID: p + "setbot prompt_bio", Text: "Bio"},
			{ID: p + "setbot reset", Text: "Reset All"},
			{ID: p + "setbot page 1", Text: "◀️ Back"},
		}
	}

	return sendInteractiveButtons(ctx, sb.String(), fmt.Sprintf("%s Settings", botName), buttons)
}

func sendWizardSummaryCard(ctx *Context) error {
	p := ctx.GetPrefix()
	botName := ctx.GetBotName()
	curPrefix := p
	if curPrefix == "" {
		curPrefix = "(none)"
	}

	thumbStatus := "Default (whatsrook.mp4)"
	if s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore); ok {
		if custom, err := s.GetSetting(ctx.Ctx, "menu_thumbnail_path"); err == nil && custom != "" {
			if _, errStat := os.Stat(custom); errStat == nil {
				thumbStatus = "Custom Thumbnail"
			}
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Bot Customization Completed!\n\n")
	fmt.Fprintf(&sb, "╭━━━〔 BOT CONFIGURATION 〕━━━\n")
	fmt.Fprintf(&sb, "│ Name      : %s\n", botName)
	fmt.Fprintf(&sb, "│ Thumbnail : %s\n", thumbStatus)
	fmt.Fprintf(&sb, "│ Prefix    : %s\n", curPrefix)
	fmt.Fprintf(&sb, "╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	fmt.Fprintf(&sb, "Type %smenu anytime to view your updated bot commands menu! (Or %sreconfigure to adjust settings)", p, p)

	return ctx.Reply(sb.String())
}
