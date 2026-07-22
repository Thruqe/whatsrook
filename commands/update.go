package commands

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/Thruqe/whatsrook/updater"
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func init() {
	Register(&Command{
		Name:        "update",
		Description: "Check for updates and show update menu or execute update",
		Category:    "updater",
		IsPublic:    false,
		Handler:     handleUpdateCommand,
	})
	Register(&Command{
		Name:        "upgrade",
		Description: "Upgrade the bot to Beta release (nightly build per commit)",
		Category:    "updater",
		IsPublic:    false,
		Handler:     handleUpgradeCommand,
	})
}

func handleUpdateCommand(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	if len(ctx.Args) > 0 {
		arg := strings.ToLower(ctx.Args[0])
		if arg == "check" || arg == "confirm" || arg == "now" {
			return performCheckAndUpdate(ctx)
		}
	}

	return sendCheckPrompt(ctx)
}

func handleUpgradeCommand(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	if len(ctx.Args) > 0 && strings.EqualFold(ctx.Args[0], "now") {
		_ = ctx.Reply("Upgrading to Beta (nightly build)...")
		return executeUpdate(ctx, true)
	}

	return sendUpgradePrompt(ctx)
}

func sendCheckPrompt(ctx *Context) error {
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
			ButtonID: new("!update check"),
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

func sendUpgradePrompt(ctx *Context) error {
	bodyText := `┌─ム ᴡʜᴀᴛsʀᴏᴏᴋ ᴜᴘɢʀᴀᴅᴇ
│ sᴡɪᴛᴄʜ ғʀᴏᴍ sᴛᴀʙʟᴇ ᴛᴏ ɴɪɢʜᴛʟʏ (ʙᴇᴛᴀ)?
╰──────────────────╯

┌─ム  sᴡɪᴛᴄʜ ᴄʜᴀɴɴᴇʟ
│
├─ム  sᴇʟᴇᴄᴛ ᴀ ʙᴜᴛᴛᴏɴ ʙᴇʟᴏᴡ
│
╰─────────◆────────╯`

	return sendButtonsMessage(ctx, bodyText, []*waE2E.ButtonsMessage_Button{
		{
			ButtonID: new("!upgrade now"),
			ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
				DisplayText: new("Switch to Nightly"),
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
			ButtonID: new("!update confirm"),
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
