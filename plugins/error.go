// Package plugins provides command registration, dispatching, and error handling utilities.
package plugins

import (
	"fmt"
	"log/slog"
)

// PluginError represents a structured, user-facing error returned by a command handler.
type PluginError struct {
	UserMessage string
	Cause       error
}

// Error implements the error interface.
func (e *PluginError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.UserMessage, e.Cause)
	}
	return e.UserMessage
}

// Unwrap returns the underlying cause error.
func (e *PluginError) Unwrap() error {
	return e.Cause
}

// Fail creates a new PluginError with a user-facing error message.
func Fail(msg string) error {
	return &PluginError{UserMessage: msg}
}

// Failf creates a formatted PluginError with a user-facing error message.
func Failf(format string, a ...any) error {
	return &PluginError{UserMessage: fmt.Sprintf(format, a...)}
}

// WrapError wraps an internal error with a clean user-facing error message.
func WrapError(userMsg string, cause error) error {
	return &PluginError{UserMessage: userMsg, Cause: cause}
}

// ErrUsage creates a standard usage error.
func ErrUsage(usage string) error {
	return Failf("Usage: %s", usage)
}

// ErrPermission creates a standard permission denied error.
func ErrPermission(msg string) error {
	if msg == "" {
		msg = "This command is restricted to sudoers/owners only."
	}
	return Fail(msg)
}

// ErrMediaRequired returns a standard media required error.
func ErrMediaRequired() error {
	return Fail("No media found in this message or the replied message.")
}

// ErrGroupOnly returns a standard group-only command error.
func ErrGroupOnly() error {
	return Fail("This command can only be used in a group chat.")
}

// logHandlerErr logs errors returned by command handlers.
func logHandlerErr(name string, err error) {
	if err == nil {
		return
	}
	slog.Error("command handler failed", "command", name, "err", err)
}

// LogHandlerErrWithContext logs errors and sends user-facing replies to the chat context.
func LogHandlerErrWithContext(cctx *Context, name string, err error) {
	if err == nil {
		return
	}

	slog.Error("command handler failed", "command", name, "err", err)

	if cctx == nil {
		return
	}

	if pErr, ok := err.(*PluginError); ok {
		_ = cctx.Reply(fmt.Sprintf("%s", pErr.UserMessage))
		return
	}

	_ = cctx.Reply(fmt.Sprintf("Command execution failed: %v", err))
}
