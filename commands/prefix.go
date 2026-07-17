package commands

import (
	"fmt"
	"strings"

	"github.com/Thruqe/whatsrook/store/sqlstore"
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
			return err
		}
		if raw == "" {
			return sendText(ctx, fmt.Sprintf("Prefix: %q (default)", DefaultPrefix))
		}
		return sendText(ctx, fmt.Sprintf("Prefix(es): %s", raw))
	}

	// Validate tokens — each must be a single printable word or the keyword "none".
	parts := strings.Fields(ctx.RawArgs)
	if len(parts) == 0 {
		return sendText(ctx, "Usage: prefix <symbol...>  (use 'none' for no prefix required)")
	}

	stored := strings.Join(parts, " ")
	if err := s.PutSetting(ctx.Ctx, PrefixSettingKey, stored); err != nil {
		return err
	}

	// Build a human-readable confirmation.
	display := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.EqualFold(p, "none") {
			display = append(display, "(no prefix)")
		} else {
			display = append(display, fmt.Sprintf("%q", p))
		}
	}
	return sendText(ctx, fmt.Sprintf("Prefix updated to: %s", strings.Join(display, ", ")))
}
