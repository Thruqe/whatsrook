// Command handler error logging utility.
package commands

import "log/slog"

func logHandlerErr(name string, err error) {
	slog.Error("command handler failed", "command", name, "err", err)
}
