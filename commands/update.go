// Update command - check for and apply pre-built release updates from GitHub.
package commands

import (
	"fmt"
	"log/slog"
	"strings"

	"whatsrook/store/sqlstore"
	"whatsrook/updater"
)

func init() {
	Register(&Command{
		Name:        "update",
		Description: "Check for updates and manage update configuration",
		Category:    "updater",
		IsPublic:    false,
		Handler:     handleUpdateCommand,
	})
	Register(&Command{
		Name:        "upgrade",
		Description: "Upgrade the bot to the latest system binary build",
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
	channel := updater.GetChannel(ctx.Ctx, s)

	if len(ctx.Args) == 0 {
		return showUpdateStatus(ctx, channel)
	}

	sub := strings.ToLower(ctx.Args[0])
	switch sub {
	case "check":
		return performCheck(ctx)
	case "stable":
		if s != nil {
			_ = updater.SetChannel(ctx.Ctx, s, "stable")
		}
		return ctx.Reply("Update channel set to stable. Run !update check to verify available releases.")
	case "beta":
		if s != nil {
			_ = updater.SetChannel(ctx.Ctx, s, "beta")
		}
		return ctx.Reply("Update channel set to beta. Run !update check to verify available releases.")
	case "channel":
		if len(ctx.Args) > 1 {
			ch := strings.ToLower(ctx.Args[1])
			if ch == "stable" || ch == "beta" {
				if s != nil {
					_ = updater.SetChannel(ctx.Ctx, s, ch)
				}
				return ctx.Reply(fmt.Sprintf("Update channel set to %s.", ch))
			}
		}
		return ctx.Reply("Usage: !update channel stable | beta")
	case "now", "confirm", "apply":
		return performUpgrade(ctx, channel == "beta")
	default:
		return showUpdateStatus(ctx, channel)
	}
}

func handleUpgradeCommand(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("You are not authorized to use this command.")
	}

	s, _ := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	channel := updater.GetChannel(ctx.Ctx, s)
	return performUpgrade(ctx, channel == "beta")
}

func showUpdateStatus(ctx *Context, channel string) error {
	currentVer, err := updater.ReadLocalVersion(updater.VersionFile)
	if err != nil {
		currentVer = "unknown"
	}

	platform := updater.GetPlatform()
	p := ctx.GetPrefix()

	msg := fmt.Sprintf(
		"WhatsRook Updater Status\nSystem: %s\nCurrent Version: %s\nChannel: %s\n\nSubcommands:\n- %supdate check: Check for new release\n- %supdate stable: Switch to stable channel\n- %supdate beta: Switch to beta channel\n- %supdate now: Apply update and restart",
		platform, currentVer, channel, p, p, p, p,
	)
	return ctx.Reply(msg)
}

func performCheck(ctx *Context) error {
	_ = ctx.Reply("Checking for system binary updates...")
	check, err := updater.CheckUpdate()
	if err != nil {
		slog.Error("update check failed", "err", err)
		return ctx.Reply(fmt.Sprintf("Update check failed: %v", err))
	}

	p := ctx.GetPrefix()
	if !check.HasNewVersion {
		return ctx.Reply(fmt.Sprintf("WhatsRook is up to date (Version %s, Platform %s).", check.CurrentVersion, check.Platform))
	}

	return ctx.Reply(fmt.Sprintf(
		"Update available!\nCurrent Version: %s\nLatest Version: %s\nPlatform: %s\n\nRun %supdate now or %supgrade to install the new binary release.",
		check.CurrentVersion, check.LatestVersion, check.Platform, p, p,
	))
}

func performUpgrade(ctx *Context, isBeta bool) error {
	channelName := "stable"
	if isBeta {
		channelName = "beta"
	}

	_ = ctx.Reply(fmt.Sprintf("Downloading %s binary release for %s...", channelName, updater.GetPlatform()))

	res, err := updater.PerformUpdate(isBeta)
	if err != nil {
		slog.Error("update execution failed", "err", err)
		return ctx.Reply(fmt.Sprintf("Update failed: %v", err))
	}

	_ = ctx.Reply(fmt.Sprintf("%s\nRestarting process now...", res.Message))

	if err := updater.RestartProcess(); err != nil {
		slog.Error("failed to restart process after update", "err", err)
		return ctx.Reply(fmt.Sprintf("Updated binary successfully, but process restart failed: %v", err))
	}

	return nil
}
