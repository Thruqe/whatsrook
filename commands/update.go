package commands

import (
	"fmt"
	"log/slog"

	"github.com/Thruqe/whatsrook/updater"
)

func init() {
	Register(&Command{
		Name:        "update",
		Description: "Update the bot to the latest release or git version",
		Category:    "settings",
		IsPublic:    false,
		Handler:     handleUpdateCommand,
	})
}

func handleUpdateCommand(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	_ = ctx.Reply("Checking for updates...")

	res, err := updater.PerformUpdate()
	if err != nil {
		slog.Error("update failed", "err", err)
		return ctx.Reply(fmt.Sprintf("Update failed: %v", err))
	}

	if !res.Updated {
		return ctx.Reply(fmt.Sprintf("Bot is already up to date. Version: %s", res.CurrentVersion))
	}

	_ = ctx.Reply(fmt.Sprintf("%s\nRestarting process now...", res.Message))

	if err := updater.RestartProcess(); err != nil {
		slog.Error("failed to restart process after update", "err", err)
		return ctx.Reply(fmt.Sprintf("Updated successfully, but process restart failed: %v", err))
	}

	return nil
}
