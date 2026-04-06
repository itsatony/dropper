package dropper

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testVersion = "0.0.1-test"

func TestNewLogger_JSONFormat(t *testing.T) {
	cfg := LoggingConfig{
		Level:  LogLevelInfo,
		Format: LogFormatJSON,
		Output: LogOutputStdout,
	}
	logger := NewLogger(cfg, testVersion)
	assert.NotNil(t, logger)
	assert.True(t, logger.Enabled(context.Background(), slog.LevelInfo))
	assert.False(t, logger.Enabled(context.Background(), slog.LevelDebug))
}

func TestNewLogger_ConsoleFormat(t *testing.T) {
	cfg := LoggingConfig{
		Level:  LogLevelDebug,
		Format: LogFormatConsole,
		Output: LogOutputStderr,
	}
	logger := NewLogger(cfg, testVersion)
	assert.NotNil(t, logger)
	assert.True(t, logger.Enabled(context.Background(), slog.LevelDebug))
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  slog.Level
	}{
		{name: "debug", input: LogLevelDebug, want: slog.LevelDebug},
		{name: "info", input: LogLevelInfo, want: slog.LevelInfo},
		{name: "warn", input: LogLevelWarn, want: slog.LevelWarn},
		{name: "error", input: LogLevelError, want: slog.LevelError},
		{name: "unknown_defaults_to_info", input: "unknown", want: slog.LevelInfo},
		{name: "empty_defaults_to_info", input: "", want: slog.LevelInfo},
		{name: "case_insensitive", input: "DEBUG", want: slog.LevelDebug},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLogLevel(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLogOutput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *os.File
	}{
		{name: "stdout", input: LogOutputStdout, want: os.Stdout},
		{name: "stderr", input: LogOutputStderr, want: os.Stderr},
		{name: "unknown_defaults_to_stdout", input: "unknown", want: os.Stdout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := logOutput(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
