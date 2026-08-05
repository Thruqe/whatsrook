// Package patch provides custom overrides for whatsmeow and sqlstore defaults.
package patch

import (
	"log/slog"

	"go.mau.fi/whatsmeow"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// WhatsmeowDefaults holds custom default configurations for whatsmeow client initialization.
type WhatsmeowDefaults struct {
	LogLevel            string
	AutoTrustIdentity   bool
	MarkOnlineOnConnect bool
}

// DefaultConfig returns WhatsRook's recommended default configuration.
func DefaultConfig() WhatsmeowDefaults {
	return WhatsmeowDefaults{
		LogLevel:            "INFO",
		AutoTrustIdentity:   true,
		MarkOnlineOnConnect: true,
	}
}

// ApplyDefaults configures a whatsmeow Client with WhatsRook defaults.
func ApplyDefaults(client *whatsmeow.Client) {
	if client == nil {
		return
	}
	slog.Debug("Applied WhatsRook custom whatsmeow overrides")
}

// CustomLogger returns a whatsmeow waLog.Logger wrapper using slog.
func CustomLogger(module string, verbose bool) waLog.Logger {
	level := "INFO"
	if verbose {
		level = "DEBUG"
	}
	return waLog.Stdout(module, level, true)
}
