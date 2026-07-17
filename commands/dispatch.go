package commands

import (
	"context"
	"strings"

	"github.com/Thruqe/whatsrook/store/sqlstore"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	// DefaultPrefix is used when no prefix has been configured in the DB.
	DefaultPrefix = "."
	// PrefixSettingKey is the bot_settings key that stores the prefix list.
	PrefixSettingKey = "prefix"
)

// Dispatch checks if the message text is a recognised command and runs it.
// Returns true if a command matched (and was handled), false otherwise.
func Dispatch(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	text := extractText(evt)
	if text == "" {
		return false
	}

	prefixes := activePrefixes(ctx, client)
	hasEmpty := false

	// Try non-empty prefixes first.
	for _, p := range prefixes {
		if p == "" {
			hasEmpty = true
			continue
		}
		if strings.HasPrefix(text, p) {
			body := strings.TrimSpace(text[len(p):])
			return runCommand(ctx, client, evt, body)
		}
	}

	// Empty prefix: treat the whole message as a potential command.
	if hasEmpty {
		return runCommand(ctx, client, evt, strings.TrimSpace(text))
	}

	return false
}

// activePrefixes returns the effective prefix list for this session.
// It reads from the DB on every call; for a personal bot the single-row
// query is negligible. Falls back to DefaultPrefix on any error.
func activePrefixes(ctx context.Context, client *whatsmeow.Client) []string {
	s, ok := client.Store.Identities.(*sqlstore.SQLStore)
	if !ok {
		return []string{DefaultPrefix}
	}
	raw, err := s.GetSetting(ctx, PrefixSettingKey)
	if err != nil || raw == "" {
		return []string{DefaultPrefix}
	}
	parts := strings.Fields(raw)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.EqualFold(p, "none") {
			out = append(out, "") // "none" → empty prefix
		} else {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{DefaultPrefix}
	}
	return out
}

// runCommand parses body (prefix already stripped) and executes the matching
// command in a goroutine. Returns false if no command matched.
func runCommand(ctx context.Context, client *whatsmeow.Client, evt *events.Message, body string) bool {
	if body == "" {
		return false
	}

	fields := strings.Fields(body)
	name := strings.ToLower(fields[0])
	args := fields[1:]

	cmd, ok := Get(name)
	if !ok {
		return false
	}

	rawArgs := ""
	if idx := strings.Index(body, fields[0]); idx == 0 {
		rawArgs = strings.TrimSpace(body[len(fields[0]):])
	}

	cctx := &Context{
		Ctx:     ctx,
		Client:  client,
		Evt:     evt,
		Command: name,
		Args:    args,
		RawArgs: rawArgs,
		Chat:    evt.Info.Chat,
		Sender:  evt.Info.Sender,
	}

	go func() {
		if err := cmd.Handler(cctx); err != nil {
			logHandlerErr(name, err)
		}
	}()

	return true
}

func extractText(evt *events.Message) string {
	if evt.Message.GetConversation() != "" {
		return evt.Message.GetConversation()
	}
	if evt.Message.GetExtendedTextMessage() != nil {
		return evt.Message.GetExtendedTextMessage().GetText()
	}
	return ""
}

