// Command handler error logging utility.
package plugins

import "log/slog"

func logHandlerErr(name string, err error) {
	slog.Error("command handler failed", "command", name, "err", err)
}
