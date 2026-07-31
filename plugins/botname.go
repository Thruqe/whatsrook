// BotName command – view or change the bot's display name.
package commands

import (
	"fmt"
	"strings"

	"whatsrook/store/sqlstore"
)

const BotNameSettingKey = "bot_name"

func init() {
	Register(&Command{
		Name:        "botname",
		Aliases:     []string{"setbotname", "setname", "name"},
		Description: "View or change the bot display name (e.g. .botname Jarvis)",
		Category:    "settings",
		Handler:     handleBotName,
	})
}

func handleBotName(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ctx.Reply("Settings store unavailable.")
	}

	// No args — show current name.
	if ctx.RawArgs == "" {
		current := ctx.GetBotName()
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Current bot name: %q\n\nTo change it:\n- %sbotname <New Name>\n\nTo reset to default:\n- %sbotname reset", current, p, p))
	}

	arg := strings.TrimSpace(ctx.RawArgs)
	if strings.EqualFold(arg, "reset") || strings.EqualFold(arg, "default") {
		if err := s.PutSetting(ctx.Ctx, BotNameSettingKey, ""); err != nil {
			return ctx.Reply("Failed to reset bot name.")
		}
		return ctx.Reply("Bot name reset to default: \"WhatsRook\".")
	}

	newName := arg
	if err := s.PutSetting(ctx.Ctx, BotNameSettingKey, newName); err != nil {
		return ctx.Reply("Failed to update bot name.")
	}

	return ctx.Reply(fmt.Sprintf("Bot name successfully updated to: %q!", newName))
}
