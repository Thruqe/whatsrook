package commands

import (
	"fmt"
	"log/slog"
	"strings"

	"whatsrook/store/sqlstore"
	"whatsrook/updater"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func init() {
	Register(&Command{
		Name:        "update",
		Description: "Check for updates and manage channel configuration",
		Category:    "updater",
		IsPublic:    false,
		Handler:     handleUpdateCommand,
	})
	Register(&Command{
		Name:        "upgrade",
		Description: "Upgrade the bot according to the selected channel (Stable / Beta)",
		Category:    "updater",
		IsPublic:    false,
		Handler:     handleUpgradeCommand,
	})
}

func handleUpdateCommand(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, _ := ctx.Client.Store.Identities.(*sqlstore.SQLStore)

	if len(ctx.Args) > 0 {
		arg := strings.ToLower(ctx.Args[0])
		if arg == "stable" {
			if s != nil {
				_ = updater.SetChannel(ctx.Ctx, s, "stable")
			}
			return ctx.Reply("Update channel set to **Stable**. Running check...")
		}
		if arg == "beta" {
			if s != nil {
				_ = updater.SetChannel(ctx.Ctx, s, "beta")
			}
			return ctx.Reply("Update channel set to **Beta**. Running check...")
		}
		if arg == "check" || arg == "confirm" || arg == "now" {
			return performCheckAndUpdate(ctx)
		}
	}

	rawCh := updater.GetChannel(ctx.Ctx, s)
	if rawCh == "" {
		return sendChannelSelectMenu(ctx)
	}

	return sendCheckPrompt(ctx)
}

func handleUpgradeCommand(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, _ := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	channel := updater.GetChannel(ctx.Ctx, s)
	isBeta := channel == "beta"

	if len(ctx.Args) > 0 && strings.EqualFold(ctx.Args[0], "now") {
		if isBeta {
			_ = ctx.Reply("Upgrading bot via Beta release (per commit nightly)...")
		} else {
			_ = ctx.Reply("Upgrading bot via Stable release...")
		}
		return executeUpdate(ctx, isBeta)
	}

	return sendUpgradePrompt(ctx, channel)
}

func sendChannelSelectMenu(ctx *Context) error {
	p := ctx.GetPrefix()
	bodyText := `┌─ム ᴡʜᴀᴛsʀᴏᴏᴋ ᴜᴘᴅᴀᴛᴇ
│ ғɪʀsᴛ ᴛɪᴍᴇ sᴇᴛᴜᴘ:
│ sᴇʟᴇᴄᴛ ʏᴏᴜʀ ᴜᴘᴅᴀᴛᴇ ᴄʜᴀɴɴᴇʟ
╰──────────────────╯

┌─ム  ᴄʜᴏᴏsᴇ ᴄʜᴀɴɴᴇʟ
│
├─ム  sᴛᴀʙʟᴇ: ᴏғғɪᴄɪᴀʟ ᴠᴇʀsɪᴏɴ ʀᴇʟᴇᴀsᴇs
├─ム  ʙᴇᴛᴀ: ɴɪɢʜᴛʟʏ ᴘᴇʀ-ᴄᴏᴍᴍɪᴛ ʙᴜɪʟᴅs
│
╰─────────◆────────╯`

	return sendButtonsMessage(ctx, bodyText, []*waE2E.ButtonsMessage_Button{
		{
			ButtonID: new(p + "update stable"),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
				DisplayText: new("Stable"),
			},
			Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
		},
		{
			ButtonID: new(p + "update beta"),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
				DisplayText: new("Beta"),
			},
			Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
		},
	})
}

func sendCheckPrompt(ctx *Context) error {
	p := ctx.GetPrefix()
	bodyText := `┌─ム ᴡʜᴀᴛsʀᴏᴏᴋ ᴜᴘᴅᴀᴛᴇ
│ ᴄʜᴇᴄᴋ ғᴏʀ ɴᴇᴡ ᴠᴇʀsɪᴏɴs?
╰──────────────────╯

┌─ム  ᴄᴏɴғɪʀᴍ ᴀᴄᴛɪᴏɴ
│
├─ム  sᴇʟᴇᴄᴛ ᴀ ʙᴜᴛᴛᴏɴ ʙᴇʟᴏᴡ
│
╰─────────◆────────╯`

	return sendButtonsMessage(ctx, bodyText, []*waE2E.ButtonsMessage_Button{
		{
			ButtonID: new(p + "update check"),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
				DisplayText: new("Continue"),
			},
			Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
		},
		{
			ButtonID: new("cancel_action"),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
				DisplayText: new("Cancel"),
			},
			Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
		},
	})
}

func performCheckAndUpdate(ctx *Context) error {
	_ = ctx.Reply("Checking for updates...")
	check, err := updater.CheckUpdate()
	if err != nil {
		slog.Error("update check failed", "err", err)
		return ctx.Reply(fmt.Sprintf("Update check failed: %v", err))
	}

	if !check.HasNewVersion {
		return ctx.Reply(fmt.Sprintf("Bot is up to date (Version %s).", check.CurrentVersion))
	}

	return sendUpdateAvailableMenu(ctx, check)
}

func sendUpgradePrompt(ctx *Context, channel string) error {
	p := ctx.GetPrefix()
	bodyText := fmt.Sprintf(`┌─ム ᴡʜᴀᴛsʀᴏᴏᴋ ᴜᴘɢʀᴀᴅᴇ
│ ᴄᴜʀʀᴇɴᴛ ᴄʜᴀɴɴᴇʟ: %s
│ ᴘʀᴏᴄᴇᴇᴅ ᴡɪᴛʜ %s ᴜᴘɢʀᴀᴅᴇ?
╰──────────────────╯

┌─ム  ᴄᴏɴғɪʀᴍ ᴜᴘɢʀᴀᴅᴇ
│
├─ム  sᴇʟᴇᴄᴛ ᴀ ʙᴜᴛᴛᴏɴ ʙᴇʟᴏᴡ
│
╰─────────◆────────╯`, strings.ToUpper(channel), strings.ToUpper(channel))

	return sendButtonsMessage(ctx, bodyText, []*waE2E.ButtonsMessage_Button{
		{
			ButtonID: new(p + "upgrade now"),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
				DisplayText: new("Upgrade Now"),
			},
			Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
		},
		{
			ButtonID: new("cancel_action"),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
				DisplayText: new("Cancel"),
			},
			Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
		},
	})
}

func sendUpdateAvailableMenu(ctx *Context, check *updater.UpdateResult) error {
	p := ctx.GetPrefix()
	bodyText := fmt.Sprintf(`┌─ム ᴡʜᴀᴛsʀᴏᴏᴋ ᴜᴘᴅᴀᴛᴇ
│ ᴄᴜʀʀᴇɴᴛ: %s
│ ʟᴀᴛᴇsᴛ: %s
│ ᴍᴇᴛʜᴏᴅ: %s
╰──────────────────╯

┌─ム  ᴜᴘᴅᴀᴛᴇ ᴀᴠᴀɪʟᴀʙʟᴇ
│
├─ム  sᴇʟᴇᴄᴛ ᴀ ʙᴜᴛᴛᴏɴ ʙᴇʟᴏᴡ
│
╰─────────◆────────╯`, check.CurrentVersion, check.LatestVersion, strings.ToUpper(check.Method))

	return sendButtonsMessage(ctx, bodyText, []*waE2E.ButtonsMessage_Button{
		{
			ButtonID: new(p + "update confirm"),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
				DisplayText: new("Update"),
			},
			Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
		},
		{
			ButtonID: new("cancel_action"),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
				DisplayText: new("Cancel"),
			},
			Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
		},
	})
}

func executeUpdate(ctx *Context, isBeta bool) error {
	res, err := updater.PerformUpdate(isBeta)
	if err != nil {
		slog.Error("update execution failed", "err", err)
		return ctx.Reply(fmt.Sprintf("Update failed: %v", err))
	}

	_ = ctx.Reply(fmt.Sprintf("%s\nRestarting process now...", res.Message))

	if err := updater.RestartProcess(); err != nil {
		slog.Error("failed to restart process after update", "err", err)
		return ctx.Reply(fmt.Sprintf("Updated successfully, but process restart failed: %v", err))
	}

	return nil
}

func sendButtonsMessage(ctx *Context, body string, buttons []*waE2E.ButtonsMessage_Button) error {
	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: new(body),
					FooterText:  new("「 Powered by WhatsRook 」"),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons:     buttons,
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
