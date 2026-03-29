package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Init configures the process-wide default logger.
//
// It is safe to call multiple times; each call replaces the default logger with
// a newly configured instance.
func Init(level string, format string) (*slog.Logger, error) {
	logger, err := New(level, format, os.Stderr)
	if err != nil {
		return nil, err
	}

	slog.SetDefault(logger)
	return logger, nil
}

// New builds a logger for the requested level and format writing to the given
// destination.
func New(level string, format string, w io.Writer) (*slog.Logger, error) {
	parsedLevel, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	handler, err := newHandler(format, w, parsedLevel)
	if err != nil {
		return nil, err
	}

	return slog.New(handler), nil
}

// WithContext returns a child logger enriched with additional structured
// attributes. If logger is nil, the current default logger is used.
func WithContext(logger *slog.Logger, args ...any) *slog.Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return logger.With(args...)
}

func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q (expected debug, info, warn, or error)", level)
	}
}

func newHandler(format string, w io.Writer, level slog.Level) (slog.Handler, error) {
	if w == nil {
		w = os.Stderr
	}

	opts := &slog.HandlerOptions{Level: level}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "text":
		return slog.NewTextHandler(w, opts), nil
	case "json":
		return slog.NewJSONHandler(w, opts), nil
	default:
		return nil, fmt.Errorf("invalid log format %q (expected json or text)", format)
	}
}
