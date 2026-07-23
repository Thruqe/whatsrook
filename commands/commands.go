// Command registry – defines the Command type, handler interface, and global
// registration table used by init() blocks across the package.
package commands

import (
	"strings"

	"whatsrook/sender"
)

// Context is passed to every command handler.
type Context = sender.Context

// Handler is the function signature for a command handler.
type Handler func(ctx *Context) error

// Command is the descriptor for a registered bot command.
type Command struct {
	Name         string
	Aliases      []string
	Description  string
	Category     string // used to group commands in the menu (e.g. "info", "downloader", "calls")
	HideFromMenu bool   // set to true for internal/helper commands that should not appear in !menu
	GroupOnly    bool   // if true, command can only be used in groups
	IsPublic     bool   // if true, command can be used by anyone; if false, restricted to sudoers/owner
	Handler      Handler
}

// CommandInfo is a plain-data description of a registered bot command,
// suitable for exposing to an AI response or external caller.
type CommandInfo struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	GroupOnly   bool     `json:"group_only"`
	IsPublic    bool     `json:"is_public"`
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

// ListCommands returns metadata for all visible (non-hidden) registered
// commands, for use by callers that need to know what commands exist
// without invoking commands.Command's unexported Handler directly.
func ListCommands() []CommandInfo {
	visible := Visible()
	out := make([]CommandInfo, 0, len(visible))
	for _, c := range visible {
		out = append(out, CommandInfo{
			Name:        c.Name,
			Aliases:     c.Aliases,
			Description: c.Description,
			Category:    c.Category,
			GroupOnly:   c.GroupOnly,
			IsPublic:    c.IsPublic,
		})
	}
	return out
}
