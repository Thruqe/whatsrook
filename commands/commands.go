package commands

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

type Handler func(ctx *Context) error

type Command struct {
	Name         string
	Aliases      []string
	Description  string
	Category     string // used to group commands in the menu (e.g. "info", "downloader", "calls")
	HideFromMenu bool   // set to true for internal/helper commands that should not appear in !menu
	Handler      Handler
}

var registry = map[string]*Command{}
var order []string // preserves registration order for help text

// Register adds a command. Call from each command file's init().
func Register(c *Command) {
	registry[c.Name] = c
	order = append(order, c.Name)
	for _, a := range c.Aliases {
		registry[a] = c
	}
}

// Get looks up a command by name or alias.
func Get(name string) (*Command, bool) {
	c, ok := registry[strings.ToLower(name)]
	return c, ok
}

// All returns commands in registration order (for a help command).
func All() []*Command {
	out := make([]*Command, 0, len(order))
	for _, name := range order {
		out = append(out, registry[name])
	}
	return out
}

// Visible returns only commands that should appear in the menu,
// deduplicated (aliases share the same *Command pointer).
func Visible() []*Command {
	seen := map[*Command]bool{}
	var out []*Command
	for _, name := range order {
		c := registry[name]
		if c.HideFromMenu || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}
