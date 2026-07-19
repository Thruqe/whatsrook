package sender

import (
	"context"

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
