// Profile commands – set bot profile picture (.pp) and group profile picture (.gpp).
package plugins

import (
	"fmt"
	"log/slog"

	"whatsrook/messaging"
	"whatsrook/utils"

	"whatsrook/wa-core/types"
)

func init() {
	Register(&Command{
		Name:        "pp",
		Aliases:     []string{"setpp", "setpfp", "botpfp"},
		Description: "Update the bot's WhatsApp profile picture (replying to an image or image upload)",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handleSetBotPP,
	})

	Register(&Command{
		Name:        "gpp",
		Aliases:     []string{"setgpp", "setgrouppfp", "grouppfp"},
		Description: "Update the group's profile picture (replying to an image or image upload in a group)",
		Category:    "group",
		GroupOnly:   true,
		IsPublic:    true,
		Handler:     handleSetGroupPP,
	})
}

func handleSetBotPP(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to bot owner and sudoers.")
	}

	downloadable, _, _ := ExtractMediaFromEvent(ctx.Evt)
	if downloadable == nil {
		return ctx.Reply(fmt.Sprintf("Please upload or reply to an image to set as profile picture. Usage: %spp", ctx.GetPrefix()))
	}

	loader := ctx.StartLoader("Downloading profile image...")
	rawBytes, err := ctx.Client.Download(ctx.Ctx, downloadable)
	loader.Delete()

	if err != nil || len(rawBytes) == 0 {
		return ctx.Reply(fmt.Sprintf("Failed to download image: %v", err))
	}

	jpegData, errConv := utils.EnsureJPEG(ctx.Ctx, rawBytes)
	if errConv != nil || len(jpegData) == 0 {
		return ctx.Reply(fmt.Sprintf("Failed to process profile image format: %v", errConv))
	}

	ownJID := types.EmptyJID
	if ctx.Client != nil && ctx.Client.Store != nil && ctx.Client.Store.ID != nil {
		ownJID = ctx.Client.Store.ID.ToNonAD()
	}

	slog.Info("handleSetBotPP: Setting bot profile picture", "rawBytes", len(rawBytes), "jpegBytes", len(jpegData), "targetJID", ownJID.String())
	picID, errSet := ctx.Client.SetGroupPhoto(ctx.Ctx, ownJID, jpegData)
	if errSet != nil {
		slog.Error("handleSetBotPP failed", "err", errSet)
		return ctx.Reply(fmt.Sprintf("Failed to update profile picture: %v", errSet))
	}

	return ctx.Reply(fmt.Sprintf("Bot profile picture updated successfully! (Picture ID: %s)", picID))
}

func handleSetGroupPP(ctx *Context) error {
	if ctx.Chat.Server != "g.us" {
		return ctx.Reply("This command can only be used in a group chat.")
	}

	// Check group info & admin permissions
	groupInfo, errGroup := ctx.Client.GetGroupInfo(ctx.Ctx, ctx.Chat)
	if errGroup == nil && groupInfo != nil {
		isBotAdmin := messaging.IsAdminRaw(ctx.Ctx, ctx.Client, groupInfo, ctx.Sender)
		if groupInfo.IsAnnounce && !isBotAdmin {
			return ctx.Reply("Only group admins are allowed to edit group info.")
		}
	}

	downloadable, _, mime := ExtractMediaFromEvent(ctx.Evt)
	if downloadable == nil {
		return ctx.Reply(fmt.Sprintf("Please upload or reply to an image to set as group profile picture. Usage: %sgpp", ctx.GetPrefix()))
	}

	loader := ctx.StartLoader("Downloading group photo...")
	rawBytes, err := ctx.Client.Download(ctx.Ctx, downloadable)
	loader.Delete()

	if err != nil || len(rawBytes) == 0 {
		return ctx.Reply(fmt.Sprintf("Failed to download image: %v", err))
	}

	jpegData, errConv := utils.EnsureJPEG(ctx.Ctx, rawBytes)
	if errConv != nil || len(jpegData) == 0 {
		return ctx.Reply(fmt.Sprintf("Failed to process group photo format: %v", errConv))
	}

	slog.Info("handleSetGroupPP: Setting group profile picture", "group", ctx.Chat.String(), "mime", mime, "rawBytes", len(rawBytes), "jpegBytes", len(jpegData))
	picID, errSet := ctx.Client.SetGroupPhoto(ctx.Ctx, ctx.Chat, jpegData)
	if errSet != nil {
		slog.Error("handleSetGroupPP failed", "err", errSet)
		return ctx.Reply(fmt.Sprintf("Failed to update group photo: %v", errSet))
	}

	return ctx.Reply(fmt.Sprintf("Group profile photo updated successfully! (Picture ID: %s)", picID))
}
