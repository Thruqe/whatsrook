// SaveContact command – save a contact to WhatsApp synced contact store via AppState SyncAction and send a native vCard.
package commands

import (
	"fmt"
	"log/slog"
	"strings"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
)

func init() {
	Register(&Command{
		Name:        "savecontact",
		Aliases:     []string{"addcontact", "savec", "contactsave"},
		Description: "Save a user to your WhatsApp contact list via AppState sync and send a native vCard",
		Category:    "user",
		IsPublic:    true,
		Handler:     handleSaveContact,
	})
}

func handleSaveContact(ctx *Context) error {
	p := ctx.GetPrefix()
	isGroup := ctx.Chat.Server == types.GroupServer

	targets := ctx.GetTargets()
	var targetJID types.JID
	if len(targets) > 0 {
		targetJID = targets[0]
	} else if !isGroup {
		targetJID = ctx.Chat
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

	if targetJID.IsEmpty() {
		return ctx.Reply(fmt.Sprintf("Please specify a user to save. Usage:\n- %ssavecontact <Name> @user\n- Reply to a message with %ssavecontact <Name>", p, p))
	}

	// Auto-detect pushname if explicit name was not provided
	if fullName == "" {
		if ctx.Evt != nil && ctx.Evt.Info.Sender.User == targetJID.User && ctx.Evt.Info.PushName != "" {
			fullName = ctx.Evt.Info.PushName
		}

		if fullName == "" && ctx.Client.Store != nil && ctx.Client.Store.Contacts != nil {
			if contact, err := ctx.Client.Store.Contacts.GetContact(ctx.Ctx, targetJID.ToNonAD()); err == nil && contact.Found {
				if contact.PushName != "" {
					fullName = contact.PushName
				} else if contact.BusinessName != "" {
					fullName = contact.BusinessName
				} else if contact.FullName != "" {
					fullName = contact.FullName
				}
			}
		}

		if fullName == "" {
			if isGroup {
				return ctx.Reply(fmt.Sprintf("Could not auto-detect contact pushname. Please specify a name:\n- %ssavecontact <Name> @user", p))
			}
			return ctx.Reply(fmt.Sprintf("Could not auto-detect contact pushname. Please specify a name:\n- %ssavecontact <Name>", p))
		}
	}

	firstName := fullName
	if len(nameParts) > 0 {
		firstName = nameParts[0]
	} else if fields := strings.Fields(fullName); len(fields) > 0 {
		firstName = fields[0]
	}

	slog.Debug("handleSaveContact: processing contact save", "target", targetJID.String(), "fullName", fullName, "firstName", firstName)

	var contactAction *waSyncAction.ContactAction
	var patch appstate.PatchInfo

	if targetJID.Server == types.HiddenUserServer {
		lidStr := targetJID.String()
		contactAction = &waSyncAction.ContactAction{
			FullName:                 new(fullName),
			FirstName:                new(firstName),
			LidJID:                   new(lidStr),
			SaveOnPrimaryAddressbook: new(true),
		}
		if ctx.Client.Store != nil && ctx.Client.Store.LIDs != nil {
			if pnJID, err := ctx.Client.Store.LIDs.GetPNForLID(ctx.Ctx, targetJID); err == nil && !pnJID.IsEmpty() {
				contactAction.PnJID = new(pnJID.ToNonAD().String())
			}
		}
		patch = appstate.PatchInfo{
			Type: appstate.WAPatchCriticalUnblockLow,
			Mutations: []appstate.MutationInfo{
				{
					Index:   []string{appstate.IndexLIDContact, lidStr},
					Version: 2,
					Value: &waSyncAction.SyncActionValue{
						ContactAction: contactAction,
					},
				},
			},
		}
	} else {
		pnStr := targetJID.ToNonAD().String()
		contactAction = &waSyncAction.ContactAction{
			FullName:                 new(fullName),
			FirstName:                new(firstName),
			PnJID:                    new(pnStr),
			SaveOnPrimaryAddressbook: new(true),
		}
		if ctx.Client.Store != nil && ctx.Client.Store.LIDs != nil {
			if lidJID, err := ctx.Client.Store.LIDs.GetLIDForPN(ctx.Ctx, targetJID); err == nil && !lidJID.IsEmpty() {
				contactAction.LidJID = new(lidJID.String())
			}
		}
		patch = appstate.PatchInfo{
			Type: appstate.WAPatchCriticalUnblockLow,
			Mutations: []appstate.MutationInfo{
				{
					Index:   []string{appstate.IndexContact, pnStr},
					Version: 2,
					Value: &waSyncAction.SyncActionValue{
						ContactAction: contactAction,
					},
				},
			},
		}
	}

	slog.Debug("handleSaveContact: sending AppState patch", "type", patch.Type, "target", targetJID.String())
	err := ctx.Client.SendAppState(ctx.Ctx, patch)
	if err != nil {
		slog.Error("handleSaveContact: failed to send AppState patch", "err", err, "target", targetJID.String())
	} else {
		slog.Debug("handleSaveContact: AppState patch sent successfully", "target", targetJID.String())
	}

	// Update local device contact store cache
	if ctx.Client.Store != nil && ctx.Client.Store.Contacts != nil {
		if err := ctx.Client.Store.Contacts.PutContactName(ctx.Ctx, targetJID.ToNonAD(), fullName, firstName); err != nil {
			slog.Error("handleSaveContact: failed to update local contact store", "err", err, "target", targetJID.String())
		} else {
			slog.Debug("handleSaveContact: updated local contact store cache", "target", targetJID.String())
		}
	}

	// Build and send native vCard ContactMessage
	vcard := fmt.Sprintf("BEGIN:VCARD\nVERSION:3.0\nN:%s;%s;;;\nFN:%s\nTEL;type=CELL;waid=%s:+%s\nEND:VCARD", firstName, fullName, fullName, targetJID.User, targetJID.User)
	vcardMsg := &waE2E.Message{
		ContactMessage: &waE2E.ContactMessage{
			DisplayName: &fullName,
			Vcard:       &vcard,
		},
	}

	_, err = ctx.Client.SendMessage(ctx.Ctx, ctx.Chat, vcardMsg)
	if err != nil {
		slog.Error("handleSaveContact: failed to send vCard message", "err", err, "chat", ctx.Chat.String())
	} else {
		slog.Debug("handleSaveContact: sent native vCard contact message", "chat", ctx.Chat.String())
	}

	resolvedJID, username := ctx.ResolveMention(targetJID)
	return ctx.ReplyWithMentions(fmt.Sprintf("Saved @%s (%s) to your WhatsApp contact sync state.", username, fullName), []types.JID{resolvedJID})
}
