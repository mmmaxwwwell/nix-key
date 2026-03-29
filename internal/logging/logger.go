// Package logging provides a structured JSON logger wrapping Go's slog.
// All output goes to stderr by default. Each log line contains:
// timestamp (ISO 8601), level, msg, module, and correlationId fields.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Config configures a Logger instance.
type Config struct {
	// Level sets the minimum log level. Messages below this level are discarded.
	Level slog.Level
	// Output is the writer for log output. Defaults to os.Stderr if nil.
	Output io.Writer
}

// Logger wraps slog.Logger with convenience methods for nix-key.
type Logger struct {
	logger *slog.Logger
}

// New creates a new Logger with the given configuration.
// If cfg.Output is nil, logs are written to stderr.
func New(cfg Config) *Logger {
	w := cfg.Output
	if w == nil {
		w = os.Stderr
	}

	level := &slog.LevelVar{}
	level.Set(cfg.Level)

	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Rename the time key to "timestamp" for spec compliance.
			if a.Key == slog.TimeKey {
				a.Key = "timestamp"
			}
			return a
		},
	})

	return &Logger{logger: slog.New(handler)}
}

// WithModule returns a child Logger that includes the given module name
// in every log entry. The parent Logger is not modified.
func (l *Logger) WithModule(module string) *Logger {
	return &Logger{logger: l.logger.With("module", module)}
}

// With returns a child Logger with additional key-value attributes.
// The parent Logger is not modified.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{logger: l.logger.With(args...)}
}

// Debug logs at DEBUG level.
func (l *Logger) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

// Info logs at INFO level.
func (l *Logger) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

// Warn logs at WARN level.
func (l *Logger) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

// Error logs at ERROR level.
func (l *Logger) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

// Slog returns the underlying *slog.Logger for interop with libraries
// that accept a standard slog logger.
func (l *Logger) Slog() *slog.Logger {
	return l.logger
}

// ParseLevel converts a level string (debug, info, warn, error) to slog.Level.
// Returns slog.LevelInfo for unrecognized values.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
