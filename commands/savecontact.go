// SaveContact command – save a contact to WhatsApp synced contact store and send a native vCard.
package commands

import (
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func init() {
	Register(&Command{
		Name:        "savecontact",
		Aliases:     []string{"addcontact", "savec", "contactsave"},
		Description: "Save a user to your WhatsApp contact list and send a native vCard",
		Category:    "user",
		IsPublic:    true,
		Handler:     handleSaveContact,
	})
}

func handleSaveContact(ctx *Context) error {
	p := ctx.GetPrefix()
	if ctx.RawArgs == "" {
		return ctx.Reply(fmt.Sprintf("Usage:\n- %ssavecontact <Name> @user\n- %ssavecontact <Name> 1234567890\n- Reply to a user's message with %ssavecontact <Name>\n\nExample:\n- %ssavecontact John Doe @1234567890", p, p, p, p))
	}

	targets := ctx.GetTargets()
	var targetJID types.JID
	if len(targets) > 0 {
		targetJID = targets[0]
	}

	args := strings.Fields(ctx.RawArgs)
	var nameParts []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "@") || strings.HasPrefix(arg, "+") {
			continue
		}
		if targetJID.User != "" && strings.Contains(arg, targetJID.User) {
			continue
		}
		nameParts = append(nameParts, arg)
	}

	fullName := strings.Join(nameParts, " ")
	if fullName == "" {
		fullName = "Contact " + targetJID.User
	}
	firstName := fullName
	if len(nameParts) > 0 {
		firstName = nameParts[0]
	}

	if targetJID.IsEmpty() {
		return ctx.Reply(fmt.Sprintf("Please specify a user to save. Usage:\n- %ssavecontact <Name> @user\n- Reply to a message with %ssavecontact <Name>", p, p))
	}

	// Check WhatsApp Privacy Settings for Contact configuration
	privacy, err := ctx.Client.TryFetchPrivacySettings(ctx.Ctx, false)
	if err != nil {
		pSettings := ctx.Client.GetPrivacySettings(ctx.Ctx)
		privacy = &pSettings
	}

	if privacy != nil {
		if privacy.GroupAdd == types.PrivacySettingNone || privacy.Messages == types.PrivacySettingNone {
			return ctx.Reply("Cannot save contact: your WhatsApp privacy settings restrict contact updates.")
		}
	}

	// Save contact name directly to WhatsApp device's contact store
	if ctx.Client.Store != nil && ctx.Client.Store.Contacts != nil {
		err := ctx.Client.Store.Contacts.PutContactName(ctx.Ctx, targetJID.ToNonAD(), fullName, firstName)
		if err != nil {
			return ctx.Reply(fmt.Sprintf("Failed to save contact to WhatsApp contact store: %v", err))
		}
	}

	// Build and send native vCard ContactMessage so user can tap "Add Contact" natively on phone
	vcard := fmt.Sprintf("BEGIN:VCARD\nVERSION:3.0\nN:%s;%s;;;\nFN:%s\nTEL;type=CELL;waid=%s:+%s\nEND:VCARD", firstName, fullName, fullName, targetJID.User, targetJID.User)
	vcardMsg := &waE2E.Message{
		ContactMessage: &waE2E.ContactMessage{
			DisplayName: &fullName,
			Vcard:       &vcard,
		},
	}

	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, vcardMsg)
	if err != nil {
		return ctx.Reply(fmt.Sprintf("Failed to send contact card: %v", err))
	}

	resolvedJID, username := ctx.ResolveMention(targetJID)
	return ctx.ReplyWithMentions(fmt.Sprintf("Saved @%s (%s) to your WhatsApp contact list.", username, fullName), []types.JID{resolvedJID})
}
