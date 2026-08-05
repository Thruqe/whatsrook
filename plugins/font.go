// Font commands – change default bot font or generate fancy text by font number (.fancy 14 text).
package plugins

import (
	"fmt"
	"strconv"
	"strings"

	"whatsrook/store/sqlstore"
	"whatsrook/utils"
)

type fontEntry struct {
	Number int
	Name   string
	Key    string
}

var indexedFonts = []fontEntry{
	{1, "Monospace", "monospace"},
	{2, "Bold", "bold"},
	{3, "Italic", "italic"},
	{4, "Bold Italic", "bold-italic"},
	{5, "Double Struck", "double-struck"},
	{6, "Script", "script"},
	{7, "Bold Script", "bold-script"},
	{8, "Fraktur", "fraktur"},
	{9, "Bold Fraktur", "bold-fraktur"},
	{10, "Sans", "sans"},
	{11, "Sans Bold", "sans-bold"},
	{12, "Sans Italic", "sans-italic"},
	{13, "Sans Bold Italic", "sans-bold-italic"},
	{14, "Small Caps", "small-caps"},
	{15, "Circled", "circled"},
	{16, "Squared", "squared"},
	{17, "Fullwidth", "fullwidth"},
	{18, "Subscript", "subscript"},
	{19, "Superscript", "superscript"},
	{20, "Parenthesized", "parenthesized"},
	{21, "Bold Sans", "bold-sans"},
	{22, "Circled Negative", "circled-negative"},
}

func init() {
	Register(&Command{
		Name:        "font",
		Description: "Switch default font style used by bot. Usage: font <style_name_or_number>",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleFont,
	})

	Register(&Command{
		Name:        "fontlist",
		Aliases:     []string{"fonts", "stylelist"},
		Description: "List all available font numbers and preview styles",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleFontList,
	})

	Register(&Command{
		Name:        "fancy",
		Aliases:     []string{"style", "fancytext", "fontconverter"},
		Description: "Convert text to a fancy font by font number. Usage: fancy <font_number> <text>",
		Category:    "tools",
		IsPublic:    true,
		Handler:     handleFancy,
	})
}

func handleFont(ctx *Context) error {
	p := ctx.GetPrefix()
	if len(ctx.Args) == 0 {
		return ctx.Reply(fmt.Sprintf("Current font style: %s\n\nUsage: %sfont <number or style_name>\nType %sfontlist to view all available font numbers.", utils.GetFontStyle(), p, p))
	}

	arg := strings.ToLower(ctx.Args[0])
	targetStyle := ""

	if num, err := strconv.Atoi(arg); err == nil && num >= 1 && num <= len(indexedFonts) {
		targetStyle = indexedFonts[num-1].Key
	} else {
		for _, f := range indexedFonts {
			if strings.EqualFold(f.Key, arg) || strings.EqualFold(f.Name, arg) {
				targetStyle = f.Key
				break
			}
		}
		if arg == "normal" || arg == "default" {
			targetStyle = "normal"
		}
	}

	if targetStyle == "" {
		return ctx.Reply(fmt.Sprintf("Invalid font style! Use %sfontlist to view valid numbers (1-%d).", p, len(indexedFonts)))
	}

	utils.SetFontStyle(targetStyle)
	if s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore); ok {
		_ = s.PutSetting(ctx.Ctx, "font_style", targetStyle)
	}

	return ctx.Reply(fmt.Sprintf("Default font style updated to *%s*.", targetStyle))
}

func handleFontList(ctx *Context) error {
	p := ctx.GetPrefix()
	sampleText := "WhatsRook Bot"

	var sb strings.Builder
	sb.WriteString("*AVAILABLE FONT NUMBERS & STYLES*\n\n")

	for _, f := range indexedFonts {
		// Save current style, convert sample text, restore current style
		curr := utils.GetFontStyle()
		utils.SetFontStyle(f.Key)
		converted := utils.ConvertFontStyle(sampleText)
		utils.SetFontStyle(curr)

		fmt.Fprintf(&sb, "*%d.* %s → %s\n", f.Number, f.Name, converted)
	}

	fmt.Fprintf(&sb, "\nUsage Examples:\n• %sfancy 14 Hello World\n• %sfont 14", p, p)
	return ctx.Reply(sb.String())
}

func handleFancy(ctx *Context) error {
	p := ctx.GetPrefix()

	if len(ctx.Args) < 2 {
		var sb strings.Builder
		sb.WriteString("Please provide a font number and text to convert.\n\n")
		sb.WriteString("Use *")
		sb.WriteString(p)
		sb.WriteString("fontlist* to view all available font numbers.\n\n")
		sb.WriteString("Usage Example:\n")
		fmt.Fprintf(&sb, "• `%sfancy 14 Hello World`\n", p)
		fmt.Fprintf(&sb, "• `%sfancy 2 WhatsRook AI`\n\n", p)
		sb.WriteString("Select an interactive font preset below to convert default sample text:")

		buttons := []struct{ ID, Text string }{
			{ID: fmt.Sprintf("%sfancy 14 Hello World", p), Text: "Small Caps (#14)"},
			{ID: fmt.Sprintf("%sfancy 2 Hello World", p), Text: "Bold (#2)"},
			{ID: fmt.Sprintf("%sfancy 8 Hello World", p), Text: "Fraktur (#8)"},
		}

		return sendInteractiveButtons(ctx, sb.String(), fmt.Sprintf("%s Fancy Converter", ctx.GetBotName()), buttons)
	}

	fontNumStr := ctx.Args[0]
	fontNum, err := strconv.Atoi(fontNumStr)
	if err != nil || fontNum < 1 || fontNum > len(indexedFonts) {
		return ctx.Reply(fmt.Sprintf("Invalid font number %q. Please choose a number between 1 and %d.\nType %sfontlist to view all font numbers.", fontNumStr, len(indexedFonts), p))
	}

	textToConvert := strings.TrimSpace(ctx.RawArgs[len(ctx.Args[0]):])
	if textToConvert == "" {
		return ctx.Reply(fmt.Sprintf("Please provide text to convert. Example: `%sfancy %d Hello World`", p, fontNum))
	}

	targetFont := indexedFonts[fontNum-1]

	curr := utils.GetFontStyle()
	utils.SetFontStyle(targetFont.Key)
	convertedText := utils.ConvertFontStyle(textToConvert)
	utils.SetFontStyle(curr)

	return ctx.Reply(convertedText)
}
