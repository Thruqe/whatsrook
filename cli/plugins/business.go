// Business commands – query WhatsApp Business profile, catalog, operating hours, business cards, and message links.
package plugins

import (
	"fmt"
	"strings"

	"whatsrook/wa-core/types"
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
	Register(&Command{
		Name:        "bizhours",
		Aliases:     []string{"businesshours", "hours"},
		Description: "View operating schedule and timezone of a WhatsApp Business account",
		Category:    "business",
		IsPublic:    true,
		Handler:     handleBusinessHours,
	})
	Register(&Command{
		Name:        "isbiz",
		Aliases:     []string{"checkbiz", "bizcheck"},
		Description: "Check if a contact or user is a WhatsApp Business account",
		Category:    "business",
		IsPublic:    true,
		Handler:     handleIsBusiness,
	})
	Register(&Command{
		Name:        "bizcard",
		Aliases:     []string{"vcardbiz", "bizcontact"},
		Description: "Display a digital Business Card summary for a WhatsApp Business account",
		Category:    "business",
		IsPublic:    true,
		Handler:     handleBusinessCard,
	})
	Register(&Command{
		Name:        "bizlink",
		Aliases:     []string{"resolvelink", "bizshortlink"},
		Description: "Resolve a WhatsApp Business short link code (wa.me/message/<code>)",
		Category:    "business",
		IsPublic:    true,
		Handler:     handleBusinessLink,
	})
}

// resolveBusinessTarget extracts the target from reply/mentions/args or current chat.
// rawTarget preserves the exact JID/LID without converting/cleaning up in mention arrays.
// queryJID resolves LID to PN if available for backend API lookups.
func resolveBusinessTarget(ctx *Context, cmdName string) (types.JID, types.JID, error) {
	targets := ctx.GetTargets()
	var rawTarget types.JID
	if len(targets) > 0 {
		rawTarget = targets[0]
	} else if ctx.Chat.Server != types.GroupServer {
		rawTarget = ctx.Chat
	} else {
		p := ctx.GetPrefix()
		return types.JID{}, types.JID{}, fmt.Errorf("Usage:\n- %s%s @user\n- %s%s 1234567890\n- Reply to a business user's message with %s%s", p, cmdName, p, cmdName, p, cmdName)
	}

	queryJID := rawTarget
	if rawTarget.Server == types.HiddenUserServer {
		if pnJID := NormalizeUserJID(ctx.Ctx, ctx.Client, rawTarget); !pnJID.IsEmpty() {
			queryJID = pnJID
		}
	} else {
		queryJID = rawTarget.ToNonAD()
	}

	return rawTarget, queryJID, nil
}

// fetchBusinessProfileAndValidate fetches the business profile and verifies the target is an actual business user.
func fetchBusinessProfileAndValidate(ctx *Context, rawTarget, queryJID types.JID) (*types.BusinessProfile, error) {
	profile, err := ctx.Client.GetBusinessProfile(ctx.Ctx, queryJID)
	if (err != nil || profile == nil) && rawTarget != queryJID {
		profile, err = ctx.Client.GetBusinessProfile(ctx.Ctx, rawTarget)
	}

	if err != nil || profile == nil {
		// Preserves rawTarget in mention array without cleaning up JID/LID
		errText := fmt.Sprintf("User @%s is not an actual WhatsApp Business account or profile is unavailable.", rawTarget.User)
		_ = ctx.ReplyWithMentions(errText, []types.JID{rawTarget})
		return nil, fmt.Errorf("not a business user")
	}

	return profile, nil
}

func handleBusinessProfile(ctx *Context) error {
	rawTarget, queryJID, err := resolveBusinessTarget(ctx, "business")
	if err != nil {
		return ctx.Reply(err.Error())
	}

	loader := ctx.StartLoader("Fetching business profile...")
	defer loader.Delete()

	profile, errFetch := fetchBusinessProfileAndValidate(ctx, rawTarget, queryJID)
	if errFetch != nil || profile == nil {
		return nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "*WhatsApp Business Profile*\n\n")
	fmt.Fprintf(&sb, "*Target:* @%s\n", rawTarget.User)

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
		if profile.BusinessHoursTimeZone != "" {
			fmt.Fprintf(&sb, "*TimeZone:* %s\n", profile.BusinessHoursTimeZone)
		}
	}

	// Preserve exact rawTarget in mention array without cleaning up JID/LID
	return ctx.ReplyWithMentions(sb.String(), []types.JID{rawTarget})
}

func handleBusinessCatalog(ctx *Context) error {
	rawTarget, queryJID, err := resolveBusinessTarget(ctx, "catalog")
	if err != nil {
		return ctx.Reply(err.Error())
	}

	loader := ctx.StartLoader("Fetching catalog...")
	defer loader.Delete()

	profile, errFetch := fetchBusinessProfileAndValidate(ctx, rawTarget, queryJID)
	if errFetch != nil || profile == nil {
		return nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "*Business Profile & Catalog Summary*\n\n")
	fmt.Fprintf(&sb, "*Target:* @%s\n", rawTarget.User)
	if profile.Email != "" {
		fmt.Fprintf(&sb, "*Email:* %s\n", profile.Email)
	}
	if profile.Address != "" {
		fmt.Fprintf(&sb, "*Address:* %s\n", profile.Address)
	}
	if len(profile.Categories) > 0 {
		cats := make([]string, len(profile.Categories))
		for i, c := range profile.Categories {
			cats[i] = c.Name
		}
		fmt.Fprintf(&sb, "*Categories:* %s\n", strings.Join(cats, ", "))
	}

	// Preserve exact rawTarget in mention array without cleaning up JID/LID
	return ctx.ReplyWithMentions(sb.String(), []types.JID{rawTarget})
}

func handleBusinessHours(ctx *Context) error {
	rawTarget, queryJID, err := resolveBusinessTarget(ctx, "bizhours")
	if err != nil {
		return ctx.Reply(err.Error())
	}

	loader := ctx.StartLoader("Fetching business hours...")
	defer loader.Delete()

	profile, errFetch := fetchBusinessProfileAndValidate(ctx, rawTarget, queryJID)
	if errFetch != nil || profile == nil {
		return nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "*Business Operating Hours*\n\n")
	fmt.Fprintf(&sb, "*Business:* @%s\n", rawTarget.User)
	if profile.BusinessHoursTimeZone != "" {
		fmt.Fprintf(&sb, "*Timezone:* %s\n", profile.BusinessHoursTimeZone)
	}
	fmt.Fprintf(&sb, "\n")

	if len(profile.BusinessHours) == 0 {
		fmt.Fprintf(&sb, "No specific business hours configured for this account.")
	} else {
		for _, bh := range profile.BusinessHours {
			day := bh.DayOfWeek
			if day == "" {
				day = "Schedule"
			}
			if bh.OpenTime != "" && bh.CloseTime != "" {
				fmt.Fprintf(&sb, "• *%s*: %s - %s (%s)\n", day, bh.OpenTime, bh.CloseTime, bh.Mode)
			} else {
				fmt.Fprintf(&sb, "• *%s*: %s\n", day, bh.Mode)
			}
		}
	}

	return ctx.ReplyWithMentions(sb.String(), []types.JID{rawTarget})
}

func handleIsBusiness(ctx *Context) error {
	rawTarget, queryJID, err := resolveBusinessTarget(ctx, "isbiz")
	if err != nil {
		return ctx.Reply(err.Error())
	}

	loader := ctx.StartLoader("Checking business status...")
	defer loader.Delete()

	profile, errProfile := ctx.Client.GetBusinessProfile(ctx.Ctx, queryJID)
	if (errProfile != nil || profile == nil) && rawTarget != queryJID {
		profile, errProfile = ctx.Client.GetBusinessProfile(ctx.Ctx, rawTarget)
	}

	if errProfile != nil || profile == nil {
		return ctx.ReplyWithMentions(fmt.Sprintf("❌ @%s is NOT an actual WhatsApp Business account.", rawTarget.User), []types.JID{rawTarget})
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "✅ @%s IS an actual WhatsApp Business account.\n", rawTarget.User)
	if len(profile.Categories) > 0 {
		cats := make([]string, len(profile.Categories))
		for i, c := range profile.Categories {
			cats[i] = c.Name
		}
		fmt.Fprintf(&sb, "*Category:* %s\n", strings.Join(cats, ", "))
	}
	if profile.Email != "" {
		fmt.Fprintf(&sb, "*Email:* %s\n", profile.Email)
	}

	return ctx.ReplyWithMentions(sb.String(), []types.JID{rawTarget})
}

func handleBusinessCard(ctx *Context) error {
	rawTarget, queryJID, err := resolveBusinessTarget(ctx, "bizcard")
	if err != nil {
		return ctx.Reply(err.Error())
	}

	loader := ctx.StartLoader("Generating business card...")
	defer loader.Delete()

	profile, errFetch := fetchBusinessProfileAndValidate(ctx, rawTarget, queryJID)
	if errFetch != nil || profile == nil {
		return nil
	}

	pushName := rawTarget.User
	if contact, cErr := ctx.Client.Store.Contacts.GetContact(ctx.Ctx, queryJID); cErr == nil && contact.Found {
		if contact.BusinessName != "" {
			pushName = contact.BusinessName
		} else if contact.PushName != "" {
			pushName = contact.PushName
		} else if contact.FullName != "" {
			pushName = contact.FullName
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "╭━━━〔 BUSINESS CARD 〕━━━\n")
	fmt.Fprintf(&sb, "│ Name     : %s\n", pushName)
	fmt.Fprintf(&sb, "│ Account  : @%s\n", rawTarget.User)

	if len(profile.Categories) > 0 {
		cats := make([]string, len(profile.Categories))
		for i, c := range profile.Categories {
			cats[i] = c.Name
		}
		fmt.Fprintf(&sb, "│ Category : %s\n", strings.Join(cats, ", "))
	}
	if profile.Email != "" {
		fmt.Fprintf(&sb, "│ Email    : %s\n", profile.Email)
	}
	if profile.Address != "" {
		fmt.Fprintf(&sb, "│ Address  : %s\n", profile.Address)
	}
	if len(profile.BusinessHours) > 0 {
		fmt.Fprintf(&sb, "│ Hours    : %d schedules (%s)\n", len(profile.BusinessHours), profile.BusinessHoursTimeZone)
	}
	fmt.Fprintf(&sb, "╰━━━━━━━━━━━━━━━━━━━━━━━━━")

	return ctx.ReplyWithMentions(sb.String(), []types.JID{rawTarget})
}

func handleBusinessLink(ctx *Context) error {
	if len(ctx.Args) == 0 {
		p := ctx.GetPrefix()
		return ctx.Reply(fmt.Sprintf("Usage:\n- %sbizlink <code>\n- %sbizlink https://wa.me/message/<code>", p, p))
	}

	rawArg := strings.TrimSpace(ctx.Args[0])
	code := rawArg

	// Extract code from URL if full URL passed
	if strings.Contains(rawArg, "wa.me/message/") {
		parts := strings.Split(rawArg, "wa.me/message/")
		if len(parts) > 1 {
			code = parts[1]
		}
	} else if strings.Contains(rawArg, "whatsapp.com/") {
		parts := strings.Split(rawArg, "/")
		code = parts[len(parts)-1]
	}
	code = strings.TrimSpace(strings.Split(code, "?")[0])

	if code == "" {
		return ctx.Reply("Invalid business short link code.")
	}

	loader := ctx.StartLoader("Resolving business message link...")
	defer loader.Delete()

	target, err := ctx.Client.ResolveBusinessMessageLink(ctx.Ctx, code)
	if err != nil || target == nil {
		return ctx.Reply(fmt.Sprintf("Could not resolve business link code %q: %v", code, err))
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "*Business Short Link Target*\n\n")
	if target.VerifiedName != "" {
		fmt.Fprintf(&sb, "*Verified Name:* %s\n", target.VerifiedName)
	}
	if target.PushName != "" {
		fmt.Fprintf(&sb, "*Push Name:* %s\n", target.PushName)
	}
	if !target.JID.IsEmpty() {
		fmt.Fprintf(&sb, "*Target Account:* @%s\n", target.JID.User)
	}
	if target.VerifiedLevel != "" {
		fmt.Fprintf(&sb, "*Verification Level:* %s\n", target.VerifiedLevel)
	}
	if target.Message != "" {
		fmt.Fprintf(&sb, "*Pre-filled Message:* %s\n", target.Message)
	}

	if !target.JID.IsEmpty() {
		return ctx.ReplyWithMentions(sb.String(), []types.JID{target.JID})
	}
	return ctx.Reply(sb.String())
}
