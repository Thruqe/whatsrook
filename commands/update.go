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
		Category:    "settings",
		IsPublic:    false,
		Handler:     handleUpdateCommand,
	})
	Register(&Command{
		Name:        "upgrade",
		Description: "Upgrade the bot to Beta release (nightly build per commit)",
		Category:    "settings",
		IsPublic:    false,
		Handler:     handleUpgradeCommand,
	})
}

func handleUpdateCommand(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	if len(ctx.Args) > 0 && strings.EqualFold(ctx.Args[0], "now") {
		return executeUpdate(ctx, false)
	}

	check, err := updater.CheckUpdate()
	if err != nil {
		slog.Error("update check failed", "err", err)
		return ctx.Reply(fmt.Sprintf("Update check failed: %v", err))
	}

	if !check.HasNewVersion {
		return ctx.Reply(fmt.Sprintf("Bot is up to date (Version %s).", check.CurrentVersion))
	}

	return sendUpdateStatusMenu(ctx, check)
}

func handleUpgradeCommand(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	_ = ctx.Reply(" Upgrading to Beta (nightly build)...")
	return executeUpdate(ctx, true)
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

func sendUpdateStatusMenu(ctx *Context, check *updater.UpdateResult) error {
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

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ButtonsMessage: &waE2E.ButtonsMessage{
					ContentText: new(bodyText),
					FooterText:  new("「 Powered by WhatsRook 」"),
					HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
					Buttons: []*waE2E.ButtonsMessage_Button{
						{
							ButtonID: new("!update now"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("Update"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
						},
						{
							ButtonID: new("cancel_update"),
							ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
								DisplayText: new("Cancel"),
							},
							Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
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
