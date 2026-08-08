// Privacy management command – view and configure WhatsApp account privacy settings using interactive buttons.
package plugins

import (
	"fmt"
	"strings"

	"whatsrook/wa-core/types"
)

func init() {
	Register(&Command{
		Name:        "privacy",
		Aliases:     []string{"privacysettings", "myprivacy"},
		Description: "View and update WhatsApp privacy settings (Last Seen, Profile Photo, Status, Read Receipts) via interactive buttons",
		Category:    "settings",
		IsPublic:    false,
		Handler:     handlePrivacy,
	})
}

func handlePrivacy(ctx *Context) error {
	if !ctx.IsSudo() {
		return ctx.Reply("Only owner/sudo users can view or modify account privacy settings.")
	}

	if len(ctx.Args) >= 2 {
		name := strings.ToLower(ctx.Args[0])
		val := strings.ToLower(ctx.Args[1])
		return updatePrivacySetting(ctx, name, val)
	}

	// Fetch current privacy settings
	loader := ctx.StartLoader("Fetching privacy settings...")
	defer loader.Delete()

	privacy, err := ctx.Client.TryFetchPrivacySettings(ctx.Ctx, false)
	if err != nil {
		pSettings := ctx.Client.GetPrivacySettings(ctx.Ctx)
		privacy = &pSettings
	}

	p := ctx.GetPrefix()
	var sb strings.Builder
	sb.WriteString("*WhatsApp Account Privacy Settings*\n\n")

	if privacy != nil {
		fmt.Fprintf(&sb, "*Last Seen:* %s\n", privacy.LastSeen)
		fmt.Fprintf(&sb, "*Profile Photo:* %s\n", privacy.Profile)
		fmt.Fprintf(&sb, "*Status:* %s\n", privacy.Status)
		fmt.Fprintf(&sb, "*Read Receipts:* %s\n", privacy.ReadReceipts)
		fmt.Fprintf(&sb, "*Group Add:* %s\n", privacy.GroupAdd)
		fmt.Fprintf(&sb, "*Online:* %s\n", privacy.Online)
		fmt.Fprintf(&sb, "*Call Add:* %s\n", privacy.CallAdd)
	} else {
		sb.WriteString("Privacy settings unavailable.\n")
	}

	sb.WriteString("\nTap a button below to configure privacy:")

	buttons := []struct{ ID, Text string }{
		{
			ID:   fmt.Sprintf("%sprivacy last all", p),
			Text: "Last Seen: Everyone",
		},
		{
			ID:   fmt.Sprintf("%sprivacy last contacts", p),
			Text: "Last Seen: Contacts",
		},
		{
			ID:   fmt.Sprintf("%sprivacy last none", p),
			Text: "Last Seen: Nobody",
		},
	}

	return sendInteractiveButtons(ctx, sb.String(), "Powered by WhatsRook", buttons)
}

func updatePrivacySetting(ctx *Context, nameStr, valStr string) error {
	var name types.PrivacySettingType
	var val types.PrivacySetting

	switch nameStr {
	case "last", "lastseen":
		name = types.PrivacySettingTypeLastSeen
	case "profile", "photo", "pfp":
		name = types.PrivacySettingTypeProfile
	case "status", "sw":
		name = types.PrivacySettingTypeStatus
	case "read", "readreceipts", "blue":
		name = types.PrivacySettingTypeReadReceipts
	case "group", "groupadd":
		name = types.PrivacySettingTypeGroupAdd
	case "online":
		name = types.PrivacySettingTypeOnline
	case "call", "calladd":
		name = types.PrivacySettingTypeCallAdd
	default:
		name = types.PrivacySettingType(nameStr)
	}

	switch valStr {
	case "everyone", "all":
		val = types.PrivacySettingAll
	case "contacts":
		val = types.PrivacySettingContacts
	case "nobody", "none":
		val = types.PrivacySettingNone
	case "match_last_seen", "matchlastseen":
		val = types.PrivacySettingMatchLastSeen
	default:
		val = types.PrivacySetting(valStr)
	}

	_, err := ctx.Client.SetPrivacySetting(ctx.Ctx, name, val)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to update privacy setting %s: %v", name, err))
	}

	return ctx.Reply(fmt.Sprintf("Successfully updated privacy setting *%s* to *%s*.", name, val))
}
