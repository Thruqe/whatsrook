package commands

import (
	"context"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

var prefixes = []string{"!", "/", "."}

// Dispatch checks if the message text is a recognized command and runs it.
// Returns true if a command matched (and was handled), false otherwise.
func Dispatch(ctx context.Context, client *whatsmeow.Client, evt *events.Message) bool {
	text := extractText(evt)
	if text == "" {
		return false
	}

	prefix, ok := matchPrefix(text)
	if !ok {
		return false
	}

	body := strings.TrimSpace(text[len(prefix):])
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
			// caller (session.go) has slog already set up as default,
			// so this is fine without threading a logger through
			logHandlerErr(name, err)
		}
	}()

	return true
}

func matchPrefix(text string) (string, bool) {
	for _, p := range prefixes {
		if strings.HasPrefix(text, p) {
			return p, true
		}
	}
	return "", false
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
