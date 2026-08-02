// Bot Customization plugin – unified setup wizard and settings for Bot Name, Menu Thumbnail, Command Prefix, and Status Bio.
package commands

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"whatsrook/meta"
	"whatsrook/store/sqlstore"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	BotNameSettingKey          = "bot_name"
	BotNamePromptDismissedKey  = "botname_prompt_dismissed"
	BotNameAwaitingInputPrefix = "botname_awaiting_input:"
)

var (
	botWizardMu        sync.RWMutex
	pendingWizardState = make(map[string]string) // key: "chatJID:senderJID" -> step ("name", "thumb", "prefix", "bio")
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
}

// HandlePendingBotCustomizationReply intercepts messages for users currently in the setup wizard or prompt sessions.
func HandlePendingBotCustomizationReply(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	if evt == nil || evt.Message == nil {
		return false
	}

	senderUser := evt.Info.Sender.ToNonAD().User
	key := fmt.Sprintf("%s:%s", evt.Info.Chat.String(), evt.Info.Sender.String())

	botWizardMu.RLock()
	step, inWizard := pendingWizardState[key]
	botWizardMu.RUnlock()

	s, okStore := client.Store.Identities.(*sqlstore.SQLStore)
	text := utils.ExtractMessageText(evt)
	imgMsg := evt.Message.GetImageMessage()
	vidMsg := evt.Message.GetVideoMessage()

	if imgMsg == nil && vidMsg == nil && evt.Message.ExtendedTextMessage != nil && evt.Message.ExtendedTextMessage.ContextInfo != nil {
		quoted := evt.Message.ExtendedTextMessage.ContextInfo.QuotedMessage
		if quoted != nil {
			imgMsg = quoted.GetImageMessage()
			vidMsg = quoted.GetVideoMessage()
		}
	}

	fakeCtx := &Context{
		Ctx:    ctx,
		Client: client,
		Chat:   evt.Info.Chat,
		Sender: evt.Info.Sender,
		Evt:    evt,
	}
	p := fakeCtx.GetPrefix()

	if !inWizard && okStore {
		rawPrompt, _ := s.GetSetting(ctx, BotNameAwaitingInputPrefix+senderUser)
		if rawPrompt == "true" && text != "" && !strings.HasPrefix(text, p) {
			step = "name"
			inWizard = true
		}
	}

	if !inWizard {
		return false
	}

	switch step {
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
		pendingWizardState[key] = "thumb"
		botWizardMu.Unlock()

		msg := fmt.Sprintf("Bot name set to %q.\n\nBot Customization Wizard (Step 2/4)\n\nPlease upload or reply with an image (.jpg/.png) or video (.mp4) to set as your bot menu thumbnail.", newName)
		_ = fakeCtx.Reply(msg)
		return true

	case "thumb":
		if imgMsg == nil && vidMsg == nil {
			_ = fakeCtx.Reply("Please upload or reply with an image (.jpg/.png) or video (.mp4) for the bot thumbnail.")
			return true
		}

		loader := fakeCtx.StartLoader("Processing custom thumbnail...")
		var data []byte
		var err error
		isVideo := false

		if vidMsg != nil {
			data, err = client.Download(ctx, vidMsg)
			isVideo = true
		} else if imgMsg != nil {
			data, err = client.Download(ctx, imgMsg)
		}
		loader.Delete()

		if err != nil || len(data) == 0 {
			_ = fakeCtx.Reply("Failed to download media for thumbnail. Please try sending another file.")
			return true
		}

		_ = os.MkdirAll("resources/songs", 0755)
		targetPath := "resources/songs/custom_menu_thumbnail.mp4"

		if isVideo {
			_ = os.WriteFile(targetPath, data, 0644)
		} else {
			tmpImg := fmt.Sprintf("/tmp/thumb_%d.jpg", time.Now().UnixNano())
			_ = os.WriteFile(tmpImg, data, 0644)
			defer os.Remove(tmpImg)

			cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-loop", "1", "-i", tmpImg, "-c:v", "libx264", "-t", "2", "-pix_fmt", "yuv420p", "-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2", targetPath)
			if err := cmd.Run(); err != nil {
				targetPath = "resources/songs/custom_menu_thumbnail.jpg"
				_ = os.WriteFile(targetPath, data, 0644)
			}
		}

		if okStore {
			_ = s.PutSetting(ctx, "menu_thumbnail_path", targetPath)
		}

		botWizardMu.Lock()
		pendingWizardState[key] = "prefix"
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
		pendingWizardState[key] = "bio"
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
		_ = client.SetStatusMessage(ctx, newBio)

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
	key := fmt.Sprintf("%s:%s", ctx.Chat.String(), ctx.Sender.String())
	args := strings.Fields(ctx.RawArgs)

	if len(args) > 0 {
		sub := strings.ToLower(args[0])

		switch sub {
		case "wizard", "setup":
			botWizardMu.Lock()
			pendingWizardState[key] = "name"
			botWizardMu.Unlock()
			return ctx.Reply("Bot Customization Wizard (Step 1/4)\n\nPlease enter your desired bot display name (e.g. Jarvis, Fuzzy, Meow):")

		case "prompt_name", "name_prompt":
			botWizardMu.Lock()
			pendingWizardState[key] = "name"
			botWizardMu.Unlock()
			return ctx.Reply("Please type your desired bot display name (e.g. Jarvis, Meow, Fuzzy):")

		case "prompt_thumb", "thumb_prompt":
			botWizardMu.Lock()
			pendingWizardState[key] = "thumb"
			botWizardMu.Unlock()
			return ctx.Reply("Please upload or reply with an image (.jpg/.png) or video (.mp4) to set as your bot menu thumbnail.")

		case "prompt_prefix", "prefix_prompt":
			botWizardMu.Lock()
			pendingWizardState[key] = "prefix"
			botWizardMu.Unlock()
			return ctx.Reply("Please send the command prefix symbol or word you want to use (e.g. ., !, / or 'none'):")

		case "prompt_bio", "bio_prompt":
			botWizardMu.Lock()
			pendingWizardState[key] = "bio"
			botWizardMu.Unlock()
			return ctx.Reply("Please send the text for your bot's WhatsApp status bio:")

		case "skip":
			stepNum := 0
			if len(args) > 1 {
				stepNum, _ = strconv.Atoi(args[1])
			}

			if stepNum == 3 {
				botWizardMu.Lock()
				pendingWizardState[key] = "bio"
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
			if err := ctx.Client.SetStatusMessage(ctx.Ctx, newBio); err != nil {
				return ctx.Reply("Failed to update status bio: " + err.Error())
			}
			return ctx.Reply("Bot status bio updated successfully!")

		case "reset":
			_ = s.PutSetting(ctx.Ctx, BotNameSettingKey, "")
			_ = s.PutSetting(ctx.Ctx, BotNamePromptDismissedKey, "")
			_ = s.PutSetting(ctx.Ctx, PrefixSettingKey, "")
			_ = s.PutSetting(ctx.Ctx, "menu_thumbnail_path", "")
			_ = os.Remove("resources/songs/custom_menu_thumbnail.mp4")
			_ = os.Remove("resources/songs/custom_menu_thumbnail.jpg")
			meta.ClearInstructionCache()
			return ctx.Reply("All bot settings (Name, Thumbnail, Prefix) reset to default values.")

		case "setup_customize":
			botWizardMu.Lock()
			pendingWizardState[key] = "name"
			botWizardMu.Unlock()
			return ctx.Reply("Bot Customization Wizard (Step 1/4)\n\nPlease enter your desired bot display name (e.g. Jarvis, Fuzzy, Meow):")

		case "setup_continue":
			bodyText := "BOT NAME CUSTOMIZATION RECOMMENDED: Keeping default name WhatsRook is not recommended. Start customization wizard below:"
			buttons := []struct{ ID, Text string }{
				{ID: p + "setbot wizard", Text: "Start Wizard"},
				{ID: p + "setbot setup_ignore", Text: "Keep Default"},
			}
			return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("Powered by %s", ctx.GetBotName()), buttons)

		case "setup_ignore":
			_ = s.PutSetting(ctx.Ctx, BotNamePromptDismissedKey, "true")
			_ = s.PutSetting(ctx.Ctx, BotNameAwaitingInputPrefix+senderUser, "")
			return ctx.Reply(fmt.Sprintf("Kept default bot name. Change anytime using %ssetbot", p))
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

	if pageNum == 1 {
		buttons = []struct{ ID, Text string }{
			{ID: p + "setbot wizard", Text: "Wizard"},
			{ID: p + "setbot prompt_name", Text: "Bot Name"},
			{ID: p + "setbot page 2", Text: "Next ▶️"},
		}
	} else if pageNum == 2 {
		buttons = []struct{ ID, Text string }{
			{ID: p + "setbot prompt_thumb", Text: "Thumbnail"},
			{ID: p + "setbot prompt_prefix", Text: "Prefix"},
			{ID: p + "setbot page 3", Text: "Next ▶️"},
		}
	} else {
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
	fmt.Fprintf(&sb, "*Bot Customization Completed!*\n\n")
	fmt.Fprintf(&sb, "╭━━━〔 BOT CONFIGURATION 〕━━━\n")
	fmt.Fprintf(&sb, "│ Name      : %s\n", botName)
	fmt.Fprintf(&sb, "│ Thumbnail : %s\n", thumbStatus)
	fmt.Fprintf(&sb, "│ Prefix    : %s\n", curPrefix)
	fmt.Fprintf(&sb, "╰━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	fmt.Fprintf(&sb, "Type %smenu anytime to view your updated bot commands menu!", p)

	return ctx.Reply(sb.String())
}
