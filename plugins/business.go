// Business command – query WhatsApp Business profile, catalog, and product details.
package plugins

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

func init() {
	Register(&Command{
		Name:        "business",
		Aliases:     []string{"biz", "bizprofile"},
		Description: "View WhatsApp Business profile details",
		Category:    "business",
		IsPublic:    true,
		Handler:     handleBusinessProfile,
	})
	Register(&Command{
		Name:        "catalog",
		Aliases:     []string{"products", "bizcatalog"},
		Description: "View WhatsApp Business catalog products of a business account",
		Category:    "business",
		IsPublic:    true,
		Handler:     handleBusinessCatalog,
	})
}

func handleBusinessProfile(ctx *Context) error {
	targets := ctx.GetTargets()
	var targetJID types.JID
	if len(targets) > 0 {
		targetJID = targets[0]
	} else if ctx.Chat.Server != types.GroupServer {
		targetJID = ctx.Chat
	} else {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sbusiness @user\n- %sbusiness 1234567890\n- Reply to a business user's message with %sbusiness", p, p, p))
	}

	profile, err := ctx.Client.GetBusinessProfile(ctx.Ctx, targetJID)
	if err != nil || profile == nil {
		return ctx.Reply(fmt.Sprintf("Could not fetch business profile for @%s. Ensure the target is a WhatsApp Business account.", targetJID.User))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "*WhatsApp Business Profile*\n\n")
	fmt.Fprintf(&sb, "*JID:* %s\n", targetJID.User)

	if len(profile.Categories) > 0 {
		cats := make([]string, len(profile.Categories))
		for i, c := range profile.Categories {
			cats[i] = c.Name
		}
		fmt.Fprintf(&sb, "*Categories:* %s\n", strings.Join(cats, ", "))
	}
	if profile.Email != "" {
		fmt.Fprintf(&sb, "*Email:* %s\n", profile.Email)
	}
	if profile.Address != "" {
		fmt.Fprintf(&sb, "*Address:* %s\n", profile.Address)
	}
	if len(profile.BusinessHours) > 0 {
		fmt.Fprintf(&sb, "*Operating Hours:* %d schedule entries\n", len(profile.BusinessHours))
	}

	return ctx.ReplyWithMentions(sb.String(), []types.JID{targetJID})
}

func handleBusinessCatalog(ctx *Context) error {
	targets := ctx.GetTargets()
	var targetJID types.JID
	if len(targets) > 0 {
		targetJID = targets[0]
	} else if ctx.Chat.Server != types.GroupServer {
		targetJID = ctx.Chat
	} else {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %scatalog @user\n- %scatalog 1234567890\n- Reply to a business user's message with %scatalog", p, p, p))
	}

	profile, err := ctx.Client.GetBusinessProfile(ctx.Ctx, targetJID)
	if err != nil || profile == nil {
		return ctx.Reply(fmt.Sprintf("No business profile or catalog available for @%s.", targetJID.User))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "*Business Profile Summary*\n\n")
	fmt.Fprintf(&sb, "*JID:* %s\n", targetJID.User)
	if profile.Email != "" {
		fmt.Fprintf(&sb, "*Email:* %s\n", profile.Email)
	}
	if profile.Address != "" {
		fmt.Fprintf(&sb, "*Address:* %s\n", profile.Address)
	}

	return ctx.ReplyWithMentions(sb.String(), []types.JID{targetJID})
}
