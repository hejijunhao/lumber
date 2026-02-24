package logging

import (
	"log/slog"
	"os"
	"strings"
)

// Init creates and sets the package-level default slog logger.
// When outputIsStdout is true, uses JSONHandler on stderr (avoids mixing with NDJSON output).
// Otherwise uses TextHandler on stderr for human readability.
func Init(outputIsStdout bool, level slog.Level) {
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if outputIsStdout {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(handler))
}

// ParseLevel converts a string ("debug", "info", "warn", "error") to slog.Level.
// Unknown strings default to LevelInfo.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
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
