// Shell command – execute arbitrary shell commands (sudo only).
package commands

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

func init() {
	Register(&Command{
		Name:        "sh",
		Aliases:     []string{"exec", "run", "shell"},
		Description: "Execute a shell command (sudoers only).",
		Category:    "system",
		IsPublic:    false,
		Handler:     handleSh,
	})
}

func handleSh(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return sendText(ctx, fmt.Sprintf("Usage: %ssh <command> [args]", ctx.GetPrefix()))
	}

	name := ctx.Args[0]
	args := ctx.Args[1:]

	output, err := runShellCmd(name, args...)
	if err != nil && output == "" {
		slog.Warn("handleSh: command error", "command", name, "args", args, "err", err)
		return sendText(ctx, "Error: "+err.Error())
	}

	if output == "" {
		output = "(no output)"
	}
	return sendText(ctx, "Output:\n```\n"+output+"\n```")
}

func runShellCmd(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
