package commands

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// allowedCommands is the fixed set of binaries the !sh command is
// permitted to execute. Only names present here can ever run — there is
// no path where arbitrary/unrecognized input reaches exec.Command.
var allowedCommands = map[string]bool{
	"uptime": true,
	"whoami": true,
	"df":     true,
	"free":   true,
	"date":   true,
}

// allowedArgs restricts which flags/arguments each allowed command may
// receive. Only exact matches are accepted. Commands with no entry here
// accept no arguments at all.
var allowedArgs = map[string][]string{
	"df":   {"-h"},
	"free": {"-h"},
}

func init() {
	Register(&Command{
		Name:        "sh",
		Description: "Run a fixed, allowlisted system command (sudoers only). Not a general shell — only pre-approved commands/args are permitted.",
		Category:    "system",
		IsPublic:    false,
		Handler:     handleSh,
	})
}

func handleSh(ctx *Context) error {
	if len(ctx.Args) == 0 {
		return sendText(ctx, fmt.Sprintf("Usage: %ssh <command> [args] — only allowlisted commands are permitted.", ctx.GetPrefix()))
	}

	name := ctx.Args[0]
	args := ctx.Args[1:]

	output, err := runAllowedCmd(name, args...)
	if err != nil {
		slog.Warn("handleSh: rejected or failed command", "command", name, "args", args, "err", err)
		return sendText(ctx, "Error: "+err.Error())
	}

	if output == "" {
		output = "(no output)"
	}
	return sendText(ctx, "```\n"+output+"\n```")
}

// runAllowedCmd executes a command from a fixed, hardcoded allowlist and
// returns its combined stdout+stderr. It never invokes a shell — argv is
// passed directly to exec.Command, so shell metacharacters (;, |, &&,
// backticks, etc.) have no special meaning. Both the command name and
// every argument must be present in the allowlists below; anything else
// is rejected before exec.Command is ever called.
func runAllowedCmd(name string, args ...string) (string, error) {
	if !allowedCommands[name] {
		return "", fmt.Errorf("command %q is not permitted", name)
	}

	permitted := allowedArgs[name]
	for _, a := range args {
		if !containsStr(permitted, a) {
			return "", fmt.Errorf("argument %q is not permitted for %q", a, name)
		}
	}

	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
