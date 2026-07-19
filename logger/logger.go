package logger

import (
	"io"
	"log/slog"
	"os"

	"github.com/rs/zerolog"
)

var LogFile *os.File

// InitLogger initializes both slog and zerolog loggers to write to stdout and "debug.log".
// If verbose is true, the levels are set to DEBUG.
func InitLogger(verbose bool) error {
	logLevel := slog.LevelInfo
	zerologLevel := zerolog.InfoLevel

	if verbose {
		logLevel = slog.LevelDebug
		zerologLevel = zerolog.DebugLevel
	}

	// Open or create the debug.log file
	var err error
	LogFile, err = os.OpenFile("debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	// Create multi-writers so logs print to console and save to debug.log
	slogWriter := io.MultiWriter(os.Stdout, LogFile)
	zerologWriter := io.MultiWriter(os.Stdout, LogFile)

	// Configure slog
	slog.SetDefault(slog.New(slog.NewTextHandler(slogWriter, &slog.HandlerOptions{Level: logLevel})))

	// Configure zerolog
	zerolog.SetGlobalLevel(zerologLevel)
	zLogger := zerolog.New(zerologWriter).With().Timestamp().Logger()
	zerolog.DefaultContextLogger = &zLogger

	return nil
}

// Close closes the open log file
func Close() {
	if LogFile != nil {
		_ = LogFile.Close()
	}
}
