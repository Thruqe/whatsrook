// Internal helpers for sending responses and retrieving configuration settings.
package commands

import (
	"context"
	"strings"

	"whatsrook/sender"
	"whatsrook/store/sqlstore"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func sendText(ctx *Context, text string) error {
	return ctx.SendText(text)
}

// sendTextRaw is like sendText but usable before a *Context exists (e.g. inside
// HandlePendingAudioReply, which runs ahead of normal command dispatch).
func sendTextRaw(ctx context.Context, client *whatsmeow.Client, chat types.JID, text string) error {
	formatted := sender.FormatTextResponseRaw(text)
	_, err := client.SendMessage(ctx, chat, &waE2E.Message{
		Conversation: &formatted,
	})
	return err
}

func getYouTubeCookie(ctx *Context) string {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return ""
	}
	cookie, _ := s.GetSetting(ctx.Ctx, CookieSettingKeyPrefix+"youtube.com")
	return cookie
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
