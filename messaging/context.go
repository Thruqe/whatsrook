package messaging

import (
	"context"
	"strings"
	"unicode"

	"whatsrook/wa-core"
	"whatsrook/wa-core/types"
	"whatsrook/wa-core/types/events"
)

// PluginContext is passed to every command handler.
type PluginContext struct {
	Ctx        context.Context
	CancelFunc context.CancelFunc
	Client     *whatsmeow.Client
	Evt        *events.Message

	Command string   // the command word itself, e.g. "ping"
	Args    []string // remaining whitespace-split args
	RawArgs string   // everything after the command, unsplit

	Chat   types.JID
	Sender types.JID
}

// Cancel invokes the context cancel function if set.
func (c *PluginContext) Cancel() {
	if c.CancelFunc != nil {
		c.CancelFunc()
	}
}

// GetSendContext returns c.Ctx if active and non-canceled, or fallback context.Background() to prevent context canceled errors.
func (c *PluginContext) GetSendContext() context.Context {
	if c == nil || c.Ctx == nil {
		return context.Background()
	}
	if err := c.Ctx.Err(); err != nil {
		return context.Background()
	}
	return c.Ctx
}

// GetPrefix returns the primary active command prefix from the database settings, or "." default.
func (c *PluginContext) GetPrefix() string {
	if c.Client == nil || c.Client.Store == nil || c.Client.Store.Identities == nil {
		return "."
	}
	s, ok := c.Client.Store.Identities.(interface {
		GetSetting(ctx context.Context, key string) (string, error)
	})
	if !ok {
		return "."
	}
	raw, err := s.GetSetting(c.Ctx, "prefix")
	if err != nil || raw == "" {
		return "."
	}
	parts := strings.Fields(raw)
	if len(parts) > 0 {
		if strings.EqualFold(parts[0], "none") || strings.EqualFold(parts[0], "empty") {
			return ""
		}
		p := parts[0]
		if isWordPrefix(p) {
			return p + " "
		}
		return p
	}
	return "."
}

func isWordPrefix(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

// GetBotName returns the configured bot display name from database settings, or "WhatsRook" default.
func (c *PluginContext) GetBotName() string {
	if c.Client == nil || c.Client.Store == nil || c.Client.Store.Identities == nil {
		return "WhatsRook"
	}
	s, ok := c.Client.Store.Identities.(interface {
		GetSetting(ctx context.Context, key string) (string, error)
	})
	if !ok {
		return "WhatsRook"
	}
	raw, err := s.GetSetting(c.Ctx, "bot_name")
	if err != nil || strings.TrimSpace(raw) == "" {
		return "WhatsRook"
	}
	return strings.TrimSpace(raw)
}

// HasArgs returns true if the command was invoked with any positional arguments.
func (c *PluginContext) HasArgs() bool {
	return len(c.Args) > 0
}

// GetArg returns the argument at the given 0-indexed position, or empty string if out of bounds.
func (c *PluginContext) GetArg(index int) string {
	if index >= 0 && index < len(c.Args) {
		return c.Args[index]
	}
	return ""
}

// GetArgOrDefault returns the argument at position index, or defaultVal if out of bounds or empty.
func (c *PluginContext) GetArgOrDefault(index int, defaultVal string) string {
	val := c.GetArg(index)
	if val == "" {
		return defaultVal
	}
	return val
}

// ReplyError sends a formatted error reply to the current chat.
func (c *PluginContext) ReplyError(msg string) error {
	return c.Reply("⚠️ " + msg)
}
