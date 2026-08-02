// Bio command – allows bot owner and sudoers to update the bot's WhatsApp status bio.
package commands

import (
	"fmt"
)

func init() {
	Register(&Command{
		Name:        "bio",
		Aliases:     []string{"setbio", "botbio", "updatebio"},
		Description: "Update the bot's WhatsApp status bio message",
		Category:    "owner",
		IsPublic:    false,
		Handler:     handleBio,
	})
}

func handleBio(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Restricted to bot owner and sudoers.")
	}

	if len(ctx.Args) == 0 {
		return ctx.Reply("Usage: .bio <new WhatsApp status bio text>\n\nExample: .bio Available | WhatsRook AI Bot")
	}

	newBio := ctx.RawArgs
	err := ctx.Client.SetStatusMessage(ctx.Ctx, newBio)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to update status bio: %v", err))
	}

	return ctx.Reply(fmt.Sprintf("Bot status bio successfully updated to:\n\"%s\"", newBio))
}
