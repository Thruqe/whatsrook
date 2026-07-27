// Initialisation of slog and zerolog loggers with configurable verbosity,
// plus a waLog.Logger-compatible adapter that mirrors whatsmeow's own
// stdout formatting while also writing to debug.log.
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// LogFile is the open file handle for debug.log, used as a log sink.
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

	// Configure slog with the whatsmeow-style handler instead of TextHandler
	slog.SetDefault(slog.New(newWMSlogHandler(slogWriter, logLevel, "App", true)))

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

// ─────────────────────────────────────────────────────────────
// whatsmeow-style formatting shared by both slog and waLog paths
// ─────────────────────────────────────────────────────────────

var wmColors = map[string]string{
	"DEBUG": "\033[90m", // gray
	"INFO":  "\033[34m", // blue
	"WARN":  "\033[33m", // yellow
	"ERROR": "\033[31m", // red
}

var wmLevelToInt = map[string]int{
	"":      -1,
	"DEBUG": 0,
	"INFO":  1,
	"WARN":  2,
	"ERROR": 3,
}

func wmFormat(mod, level, msg string, color bool) string {
	var colorStart, colorReset string
	if color {
		colorStart = wmColors[level]
		colorReset = "\033[0m"
	}
	return fmt.Sprintf("%s%s [%s %s] %s%s\n",
		time.Now().Format("15:04:05.000"), colorStart, mod, level, msg, colorReset)
}

// ─────────────────────────────────────────────────────────────
// waLog.Logger adapter (used for whatsmeow's Database/Client loggers)
// ─────────────────────────────────────────────────────────────

// WhatsmeowStyle returns a waLog.Logger-compatible logger that reproduces
// whatsmeow's own Stdout() formatting (timestamp, "[mod LEVEL]" bracket,
// ANSI colors), but writes to both stdout and debug.log via the
// multiwriter set up in InitLogger.
func WhatsmeowStyle(module string, minLevel string, color bool) *wmLogger {
	return &wmLogger{
		mod:   module,
		color: color,
		min:   wmLevelToInt[strings.ToUpper(minLevel)],
	}
}

type wmLogger struct {
	mod   string
	color bool
	min   int
}

func (w *wmLogger) outputf(level, msg string, args ...any) {
	if wmLevelToInt[level] < w.min {
		return
	}
	line := wmFormat(w.mod, level, fmt.Sprintf(msg, args...), w.color)

	dest := io.Writer(os.Stdout)
	if LogFile != nil {
		dest = io.MultiWriter(os.Stdout, LogFile)
	}
	fmt.Fprint(dest, line)
}

func (w *wmLogger) Errorf(msg string, args ...any) { w.outputf("ERROR", msg, args...) }
func (w *wmLogger) Warnf(msg string, args ...any)  { w.outputf("WARN", msg, args...) }
func (w *wmLogger) Infof(msg string, args ...any)  { w.outputf("INFO", msg, args...) }
func (w *wmLogger) Debugf(msg string, args ...any) { w.outputf("DEBUG", msg, args...) }

func (w *wmLogger) Sub(mod string) waLog.Logger {
	return &wmLogger{mod: fmt.Sprintf("%s/%s", w.mod, mod), color: w.color, min: w.min}
}

// ─────────────────────────────────────────────────────────────
// slog.Handler adapter (used for your own app's slog.Info/Debug/etc calls)
// ─────────────────────────────────────────────────────────────

// wmSlogHandler implements slog.Handler using the same bracketed,
// colorized whatsmeow-style format as wmLogger.
type wmSlogHandler struct {
	w     io.Writer
	level slog.Level
	mod   string
	color bool
	attrs []slog.Attr
}

func newWMSlogHandler(w io.Writer, level slog.Level, mod string, color bool) *wmSlogHandler {
	return &wmSlogHandler{w: w, level: level, mod: mod, color: color}
}

func (h *wmSlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *wmSlogHandler) Handle(_ context.Context, r slog.Record) error {
	levelStr := slogLevelToWM(r.Level)

	msg := r.Message
	r.Attrs(func(a slog.Attr) bool {
		msg += fmt.Sprintf(" %s=%v", a.Key, a.Value.Any())
		return true
	})
	for _, a := range h.attrs {
		msg += fmt.Sprintf(" %s=%v", a.Key, a.Value.Any())
	}

	line := wmFormat(h.mod, levelStr, msg, h.color)
	_, err := fmt.Fprint(h.w, line)
	return err
}

func (h *wmSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &wmSlogHandler{w: h.w, level: h.level, mod: h.mod, color: h.color, attrs: append(h.attrs, attrs...)}
}

func (h *wmSlogHandler) WithGroup(_ string) slog.Handler {
	return h // groups not supported; flatten
}

func slogLevelToWM(l slog.Level) string {
	switch {
	case l < slog.LevelInfo:
		return "DEBUG"
	case l < slog.LevelWarn:
		return "INFO"
	case l < slog.LevelError:
		return "WARN"
	default:
		return "ERROR"
	}
}
