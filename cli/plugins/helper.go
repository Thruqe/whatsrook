// Internal helpers for sending responses and retrieving configuration settings.
package plugins

import (
	"context"
	"strings"

	"whatsrook/messaging"
	"whatsrook/wa-core/store/sqlstore"

	"whatsrook/wa-core"
	"whatsrook/wa-core/proto/waE2E"
	"whatsrook/wa-core/types"
)

func sendText(ctx *Context, text string) error {
	return ctx.SendText(text)
}

// sendTextRaw is like sendText but usable before a *Context exists (e.g. inside
// HandlePendingAudioReply, which runs ahead of normal command dispatch).
func sendTextRaw(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string) error {
	formatted := messaging.FormatTextResponseRaw(text)
	_, err := client.SendMessage(ctx, chat, &waE2E.Message{
		Conversation: &formatted,
	})
	return err
}

func isWordPrefix(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return true
		}
	}
	return false
}

func GetBotName(ctx context.Context, client *whatsmeow.Client) string {
	if client == nil || client.Store == nil || client.Store.Identities == nil {
		return "WhatsRook"
	}
	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return "WhatsRook"
	}
	raw, err := s.GetSetting(ctx, "bot_name")
	if err != nil || strings.TrimSpace(raw) == "" {
		return "WhatsRook"
	}
	return strings.TrimSpace(raw)
}

func NormalizeUserJID(ctx context.Context, client *whatsmeow.Client, jid types.JID) types.JID {
	clean := jid.ToNonAD()
	if clean.Server == types.HiddenUserServer && client != nil && client.Store != nil && client.Store.LIDs != nil {
		if pn, err := client.Store.LIDs.GetPNForLID(ctx, clean); err == nil && !pn.IsEmpty() {
			return pn.ToNonAD()
		}
	}
	return clean
}
