package commands

import (
	"fmt"
	"strings"

	"github.com/Thruqe/whatsrook/font"
	"github.com/Thruqe/whatsrook/store/sqlstore"
)

func init() {
	Register(&Command{
		Name:        "font",
		Description: "Switch the type of font the bot uses. Usage: font [monospace/bold/script/normal]",
		Category:    "info",
		IsPublic:    false,
		Handler:     handleFont,
	})
}

func handleFont(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply(fmt.Sprintf("Usage: font [monospace/bold/script/normal]. Current font: %s", font.GetStyle()))
	}

	style := strings.ToLower(ctx.Args[0])
	switch style {
	case "monospace", "bold", "script", "normal":
		font.SetStyle(style)
		s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
		if ok {
			_ = s.PutSetting(ctx.Ctx, "font_style", style)
		}
		return ctx.Reply(fmt.Sprintf("Font style switched to %s successfully.", style))
	default:
		return ctx.Reply("Invalid font style! Choose from: monospace, bold, script, normal.")
	}
}
