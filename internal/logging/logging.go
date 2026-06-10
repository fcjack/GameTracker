package logging

import (
	"log/slog"
	"os"
	"strings"
)

var defaultLogger = slog.Default()

// Init configures the process-wide slog logger from LOG_LEVEL and LOG_FORMAT.
// LOG_LEVEL: debug, info, warn, error (default: info).
// LOG_FORMAT: json or text (default: text).
func Init() *slog.Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT"))) {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
	return defaultLogger
}

func Default() *slog.Logger {
	return defaultLogger
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
