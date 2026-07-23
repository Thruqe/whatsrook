// Internal helpers for sending responses and retrieving configuration settings.
package commands

import (
	"context"

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
	cookie, _ := s.GetSetting(ctx.Ctx, "youtube_cookie")
	return cookie
}
