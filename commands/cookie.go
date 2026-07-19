package commands

import (
	"strings"

	"github.com/Thruqe/whatsrook/store/sqlstore"
)

func init() {
	Register(&Command{
		Name:        "setcookie",
		Description: "Set the YouTube cookie for the Ember API",
		Category:    "settings",
		IsPublic:    false,
		Handler:     handleSetCookie,
	})

	Register(&Command{
		Name:        "getcookie",
		Description: "Get the stored YouTube cookie",
		Category:    "settings",
		IsPublic:    false,
		Handler:     handleGetCookie,
	})

	Register(&Command{
		Name:        "delcookie",
		Description: "Delete the stored YouTube cookie",
		Category:    "settings",
		IsPublic:    false,
		Handler:     handleDelCookie,
	})
}

func handleSetCookie(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return sendText(ctx, "Settings store unavailable.")
	}

	if ctx.RawArgs == "" {
		return sendText(ctx, "Usage: !setcookie <cookie_content>")
	}

	cookie := strings.TrimSpace(ctx.RawArgs)
	if err := s.PutSetting(ctx.Ctx, "youtube_cookie", cookie); err != nil {
		return sendText(ctx, "Failed to save cookie: "+err.Error())
	}

	return sendText(ctx, "YouTube cookie saved successfully.")
}

func handleGetCookie(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return sendText(ctx, "Settings store unavailable.")
	}

	cookie, err := s.GetSetting(ctx.Ctx, "youtube_cookie")
	if err != nil {
		return sendText(ctx, "Failed to retrieve cookie: "+err.Error())
	}

	if cookie == "" {
		return sendText(ctx, "No YouTube cookie stored.")
	}

	return sendText(ctx, "YouTube Cookie:\n"+cookie)
}

func handleDelCookie(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return sendText(ctx, "Settings store unavailable.")
	}

	if err := s.DeleteSetting(ctx.Ctx, "youtube_cookie"); err != nil {
		return sendText(ctx, "Failed to delete cookie: "+err.Error())
	}

	return sendText(ctx, "YouTube cookie deleted successfully.")
}
