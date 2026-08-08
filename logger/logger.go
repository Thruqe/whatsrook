package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	waLog "whatsrook/wa-core/util/log"
)

var levelFiles = make(map[string]*os.File)

// InitLogger initializes logging for the specified session directory, routing each
// log level into its own level.log file inside sessionDir/logs/
func InitLogger(sessionDir string, verbose bool) error {
	logDir := filepath.Join(sessionDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create logs directory: %w", err)
	}

	levels := []string{"debug", "info", "warn", "error"}
	for _, lvl := range levels {
		f, err := os.OpenFile(filepath.Join(logDir, lvl+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			Close()
			return err
		}
		levelFiles[lvl] = f
	}

	minLevel := slog.LevelInfo
	if verbose {
		minLevel = slog.LevelDebug
	}

	slog.SetDefault(slog.New(newWMSlogHandler(os.Stdout, minLevel, "App", true)))
	return nil
}

// Close closes all open per-level log files.
func Close() {
	for lvl, f := range levelFiles {
		if f != nil {
			_ = f.Close()
		}
		delete(levelFiles, lvl)
	}
}

func writeToLevelLog(level, line string) {
	if f, ok := levelFiles[strings.ToLower(level)]; ok && f != nil {
		_, _ = fmt.Fprint(f, line)
	}
}

// ─────────────────────────────────────────────────────────────
// whatsmeow-style formatting
// ─────────────────────────────────────────────────────────────

var wmColors = map[string]string{
	"DEBUG": "\033[90m",
	"INFO":  "\033[34m",
	"WARN":  "\033[33m",
	"ERROR": "\033[31m",
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
// waLog.Logger adapter
// ─────────────────────────────────────────────────────────────

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
	fmt.Fprint(os.Stdout, line)
	writeToLevelLog(level, line)
}

func (w *wmLogger) Errorf(msg string, args ...any) { w.outputf("ERROR", msg, args...) }
func (w *wmLogger) Warnf(msg string, args ...any)  { w.outputf("WARN", msg, args...) }
func (w *wmLogger) Infof(msg string, args ...any)  { w.outputf("INFO", msg, args...) }
func (w *wmLogger) Debugf(msg string, args ...any) { w.outputf("DEBUG", msg, args...) }

func (w *wmLogger) Sub(mod string) waLog.Logger {
	return &wmLogger{mod: fmt.Sprintf("%s/%s", w.mod, mod), color: w.color, min: w.min}
}

// ─────────────────────────────────────────────────────────────
// slog.Handler adapter
// ─────────────────────────────────────────────────────────────

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
	writeToLevelLog(levelStr, line)

	return err
}

func (h *wmSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &wmSlogHandler{w: h.w, level: h.level, mod: h.mod, color: h.color, attrs: append(h.attrs, attrs...)}
}

func (h *wmSlogHandler) WithGroup(_ string) slog.Handler {
	return h
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
