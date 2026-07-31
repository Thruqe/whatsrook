// BotName command – view or change the bot's display name.
package commands

import (
	"fmt"
	"strings"

	"whatsrook/meta"
	"whatsrook/store/sqlstore"
)

const (
	BotNameSettingKey          = "bot_name"
	BotNamePromptDismissedKey = "botname_prompt_dismissed"
	BotNameAwaitingInputPrefix = "botname_awaiting_input:"
)

func init() {
	Register(&Command{
		Name:        "botname",
		Aliases:     []string{"setbotname", "setname", "name"},
		Description: "View or change the bot display name (e.g. .botname Jarvis)",
		Category:    "settings",
		IsPublic:    true,
		Handler:     handleBotName,
	})
}

func handleBotName(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	p := ctx.GetPrefix()
	senderUser := ctx.Sender.ToNonAD().User

	// No args — show current name.
	if ctx.RawArgs == "" {
		current := ctx.GetBotName()
		return ctx.Reply(fmt.Sprintf("Current bot name: %q\n\nTo change it:\n- %sbotname <New Name>\n\nTo reset to default:\n- %sbotname reset", current, p, p))
	}

	arg := strings.TrimSpace(ctx.RawArgs)

	if strings.EqualFold(arg, "setup_customize") {
		_ = s.PutSetting(ctx.Ctx, BotNameAwaitingInputPrefix+senderUser, "true")
		return ctx.Reply("Please type your desired bot name (e.g. Fuzzy, Jarvis, Meow):")
	}

	if strings.EqualFold(arg, "setup_continue") {
		bodyText := "⚠️ *RECOMMENDED*: Please note that keeping the bot name as WhatsRook will have its consequences. It is strongly recommended to name it something different and unique."
		buttons := []struct{ ID, Text string }{
			{ID: p + "botname setup_customize", Text: "Customize Bot"},
			{ID: p + "botname setup_ignore", Text: "Ignore"},
		}
		return sendInteractiveButtons(ctx, bodyText, fmt.Sprintf("Powered by %s", ctx.GetBotName()), buttons)
	}

	if strings.EqualFold(arg, "setup_ignore") {
		_ = s.PutSetting(ctx.Ctx, BotNamePromptDismissedKey, "true")
		_ = s.PutSetting(ctx.Ctx, BotNameAwaitingInputPrefix+senderUser, "")
		return ctx.Reply(fmt.Sprintf("You have chosen to keep the default bot name. You can change it anytime later using the %sbotname command.", p))
	}

	if strings.EqualFold(arg, "reset") || strings.EqualFold(arg, "default") {
		if err := s.PutSetting(ctx.Ctx, BotNameSettingKey, ""); err != nil {
			return ctx.Reply("Failed to reset bot name.")
		}
		_ = s.PutSetting(ctx.Ctx, BotNamePromptDismissedKey, "")
		meta.ClearInstructionCache()
		return ctx.Reply("Bot name reset to default: \"WhatsRook\".")
	}

	newName := arg
	if err := s.PutSetting(ctx.Ctx, BotNameSettingKey, newName); err != nil {
		return ctx.Reply("Failed to update bot name.")
	}
	_ = s.PutSetting(ctx.Ctx, BotNamePromptDismissedKey, "true")
	_ = s.PutSetting(ctx.Ctx, BotNameAwaitingInputPrefix+senderUser, "")
	meta.ClearInstructionCache()

	return ctx.Reply(fmt.Sprintf("Bot name successfully updated to: %q!\n\nYou can change it anytime later using the %sbotname command.", newName, p))
}
