package plugins

import (
	"fmt"
	"strings"

	"whatsrook/store/sqlstore"
)

func init() {
	Register(&Command{
		Name:        "prefix",
		Description: "View or change the bot command prefix(es). Use 'none' for no prefix.",
		Category:    "settings",
		Handler:     handlePrefix,
	})
}

func handlePrefix(ctx *Context) error {
	s, ok := ctx.Client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return sendText(ctx, "Settings store unavailable.")
	}

	// No args — show current configuration.
	if ctx.RawArgs == "" {
		raw, err := s.GetSetting(ctx.Ctx, PrefixSettingKey)
		if err != nil {
			return sendText(ctx, "Failed to retrieve prefix configuration.")
		}
		if raw == "" {
			return sendText(ctx, fmt.Sprintf("Prefix: %q (default)", DefaultPrefix))
		}
		return sendText(ctx, fmt.Sprintf("Prefix(es): %s", raw))
	}

	parts := strings.Fields(ctx.RawArgs)
	if len(parts) == 0 {
		return sendText(ctx, "Usage: prefix <symbol or word...> (use 'empty' or 'none' for no prefix required)")
	}

	var parsedParts []string
	for _, p := range parts {
		if strings.EqualFold(p, "none") || strings.EqualFold(p, "empty") {
			parsedParts = append(parsedParts, "empty")
		} else if isWordPrefix(p) {
			parsedParts = append(parsedParts, p)
		} else {
			// Pure symbol string: if single symbol keep it, if multi-symbol like ".!" split them.
			for _, r := range p {
				parsedParts = append(parsedParts, string(r))
			}
		}
	}

	stored := strings.Join(parsedParts, " ")
	if err := s.PutSetting(ctx.Ctx, PrefixSettingKey, stored); err != nil {
		return sendText(ctx, "Failed to update prefix configuration.")
	}

	// Build a human-readable confirmation.
	display := make([]string, 0, len(parsedParts))
	for _, p := range parsedParts {
		if p == "empty" {
			display = append(display, "(no prefix)")
		} else {
			display = append(display, fmt.Sprintf("%q", p))
		}
	}
	return sendText(ctx, fmt.Sprintf("Prefix updated to: %s", strings.Join(display, ", ")))
}
