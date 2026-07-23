// Command context type that is passed to every command handler.
package sender

import (
	"context"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// Context is passed to every command handler.
type Context struct {
	Ctx    context.Context
	Client *whatsmeow.Client
	Evt    *events.Message

	Command string   // the command word itself, e.g. "ping"
	Args    []string // remaining whitespace-split args
	RawArgs string   // everything after the command, unsplit

	Chat   types.JID
	Sender types.JID
}

// GetPrefix returns the primary active command prefix from the database settings, or "." default.
func (c *Context) GetPrefix() string {
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
		return parts[0]
	}
	return "."
}
