package dropper

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestConfig writes YAML content to a temp file and returns the path.
func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "dropper.yaml")
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	return path
}

const validFullConfig = `
dropper:
  listen_port: 9090
  secret: "test-secret-value-long-enough"
  session_ttl: "12h"
  rate_limit_login: 10
  root_dir: "/tmp"
  readonly: true
  max_upload_bytes: 52428800
  allowed_extensions: [".png", ".jpg"]
  audit_log_path: "/tmp/audit.log"
  logging:
    level: "debug"
    format: "console"
    output: "stderr"
    no_log_paths: ["/healthz"]
`

const minimalConfig = `
dropper:
  secret: "test-secret-minimum-length"
  root_dir: "/tmp"
`

func TestLoadConfig_ValidFull(t *testing.T) {
	path := writeTestConfig(t, validFullConfig)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, 9090, cfg.Dropper.ListenPort)
	assert.Equal(t, "test-secret-value-long-enough", cfg.Dropper.Secret)
	assert.Equal(t, "12h", cfg.Dropper.SessionTTL)
	assert.Equal(t, 10, cfg.Dropper.RateLimitLogin)
	assert.Equal(t, "/tmp", cfg.Dropper.RootDir)
	assert.True(t, cfg.Dropper.Readonly)
	assert.Equal(t, int64(52428800), cfg.Dropper.MaxUploadBytes)
	assert.Equal(t, []string{".png", ".jpg"}, cfg.Dropper.AllowedExtensions)
	assert.Equal(t, "/tmp/audit.log", cfg.Dropper.AuditLogPath)
	assert.Equal(t, "debug", cfg.Dropper.Logging.Level)
	assert.Equal(t, "console", cfg.Dropper.Logging.Format)
	assert.Equal(t, "stderr", cfg.Dropper.Logging.Output)
	assert.Equal(t, []string{"/healthz"}, cfg.Dropper.Logging.NoLogPaths)
}

func TestLoadConfig_Defaults(t *testing.T) {
	path := writeTestConfig(t, minimalConfig)
	cfg, err := LoadConfig(path)
	require.NoError(t, err)

	assert.Equal(t, DefaultListenPort, cfg.Dropper.ListenPort)
	assert.Equal(t, DefaultSessionTTL, cfg.Dropper.SessionTTL)
	assert.Equal(t, DefaultRateLimitLogin, cfg.Dropper.RateLimitLogin)
	assert.Equal(t, int64(DefaultMaxUploadBytes), cfg.Dropper.MaxUploadBytes)
	assert.Equal(t, DefaultAuditLogPath, cfg.Dropper.AuditLogPath)
	assert.Equal(t, DefaultLogLevel, cfg.Dropper.Logging.Level)
	assert.Equal(t, DefaultLogFormat, cfg.Dropper.Logging.Format)
	assert.Equal(t, DefaultLogOutput, cfg.Dropper.Logging.Output)
	assert.False(t, cfg.Dropper.Readonly)
}

func TestLoadConfig_MissingSecret(t *testing.T) {
	yaml := `
dropper:
  root_dir: "/tmp"
`
	path := writeTestConfig(t, yaml)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrMsgConfigValidation)
}

func TestLoadConfig_ShortSecret(t *testing.T) {
	yaml := `
dropper:
  secret: "short"
  root_dir: "/tmp"
`
	path := writeTestConfig(t, yaml)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrMsgConfigValidation)
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	yaml := `
dropper:
  secret: "test-secret-minimum-length"
  root_dir: "/tmp"
  listen_port: 99999
`
	path := writeTestConfig(t, yaml)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrMsgConfigValidation)
}

func TestLoadConfig_InvalidLogLevel(t *testing.T) {
	yaml := `
dropper:
  secret: "test-secret-minimum-length"
  root_dir: "/tmp"
  logging:
    level: "trace"
    format: "json"
    output: "stdout"
`
	path := writeTestConfig(t, yaml)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrMsgConfigValidation)
}

func TestLoadConfig_InvalidSessionTTL(t *testing.T) {
	yaml := `
dropper:
  secret: "test-secret-minimum-length"
  root_dir: "/tmp"
  session_ttl: "not-a-duration"
`
	path := writeTestConfig(t, yaml)
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session_ttl")
}

func TestLoadConfig_EnvOverride(t *testing.T) {
	path := writeTestConfig(t, minimalConfig)
	t.Setenv(EnvListenPort, "9090")

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, 9090, cfg.Dropper.ListenPort)
}

func TestLoadConfig_SecretFromEnv(t *testing.T) {
	yaml := `
dropper:
  root_dir: "/tmp"
`
	path := writeTestConfig(t, yaml)
	t.Setenv(EnvSecret, "env-secret-long-enough")

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "env-secret-long-enough", cfg.Dropper.Secret)
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/dropper.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrMsgConfigLoad)
}

func TestSessionTTLDuration(t *testing.T) {
	tests := []struct {
		name    string
		ttl     string
		want    time.Duration
		wantErr bool
	}{
		{name: "24h", ttl: "24h", want: 24 * time.Hour},
		{name: "30m", ttl: "30m", want: 30 * time.Minute},
		{name: "1h30m", ttl: "1h30m", want: 90 * time.Minute},
		{name: "invalid", ttl: "invalid", wantErr: true},
		{name: "empty", ttl: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &DropperConfig{SessionTTL: tt.ttl}
			got, err := cfg.SessionTTLDuration()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
