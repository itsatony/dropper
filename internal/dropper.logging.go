package dropper

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger creates a configured slog.Logger based on LoggingConfig.
func NewLogger(cfg LoggingConfig) *slog.Logger {
	level := parseLogLevel(cfg.Level)
	opts := &slog.HandlerOptions{Level: level}

	output := logOutput(cfg.Output)

	var handler slog.Handler
	switch strings.ToLower(cfg.Format) {
	case LogFormatJSON:
		handler = slog.NewJSONHandler(output, opts)
	default:
		handler = slog.NewTextHandler(output, opts)
	}

	return slog.New(handler).With(
		LogFieldService, ServiceName,
	)
}

// logOutput returns the io.Writer for the configured output destination.
func logOutput(output string) *os.File {
	switch strings.ToLower(output) {
	case LogOutputStderr:
		return os.Stderr
	default:
		return os.Stdout
	}
}

// parseLogLevel maps string level names to slog.Level values.
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case LogLevelDebug:
		return slog.LevelDebug
	case LogLevelWarn:
		return slog.LevelWarn
	case LogLevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
