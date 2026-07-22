package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Thruqe/whatsrook/font"
	"github.com/Thruqe/whatsrook/store/sqlstore"
)

var validStyles = map[string]bool{
	"monospace":          true,
	"bold":               true,
	"italic":             true,
	"bold-italic":        true,
	"double-struck":      true,
	"script":             true,
	"bold-script":        true,
	"fraktur":            true,
	"bold-fraktur":       true,
	"sans":               true,
	"sans-bold":          true,
	"sans-italic":        true,
	"sans-bold-italic":   true,
	"circled":            true,
	"circled-negative":   true,
	"squared":            true,
	"squared-negative":   true,
	"fullwidth":          true,
	"small-caps":         true,
	"subscript":          true,
	"superscript":        true,
	"parenthesized":      true,
	"bold-sans":          true,
	"regional-indicator": true,
	"bold-script-alt":    true,
	"sans-serif-bold":    true,
	"monospace-bold":     true,
	"double-struck-bold": true,
	"circled-bold":       true,
	"squared-bold":       true,
	"small-caps-alt":     true,
	"normal":             true,
}

func init() {
	Register(&Command{
		Name:        "font",
		Description: "Switch the type of font the bot uses. Usage: font <style>",
		Category:    "tools",
		IsPublic:    false,
		Handler:     handleFont,
	})
	Register(&Command{
		Name:        "fontlist",
		Description: "List all available font styles and preview them.",
		Category:    "tools",
		IsPublic:    false,
		Handler:     handleFontList,
	})
}

func handleFont(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return ctx.Reply(fmt.Sprintf("Usage: font <style>. Current font: %s", font.GetStyle()))
	}

	style := strings.ToLower(ctx.Args[0])
	if style == "normal" {
		style = "default"
	}

	if !validStyles[style] && style != "default" {
		return ctx.Reply("Invalid font style! Use 'fontlist' command to view available options.")
	}

	font.SetStyle(style)
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if ok {
		_ = s.PutSetting(ctx.Ctx, "font_style", style)
	}

	return ctx.Reply(fmt.Sprintf("Font style switched to %s successfully.", style))
}

func handleFontList(ctx *Context) error {
	styles := make([]string, 0, len(validStyles))
	for style := range validStyles {
		styles = append(styles, style)
	}
	sort.Strings(styles)

	current := font.GetStyle()
	var sb strings.Builder
	sb.WriteString("Available Font Styles\n\n")

	for _, style := range styles {
		if style == current {
			fmt.Fprintf(&sb, "• %s (active)\n", style)
		} else {
			fmt.Fprintf(&sb, "• %s\n", style)
		}
	}

	return ctx.Reply(sb.String())
}
