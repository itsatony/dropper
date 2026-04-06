package dropper

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/itsatony/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testVersionsYAML = `manifest_version: "1.0"
project:
  name: "dropper-test"
  version: "0.0.1-test"
`

func testConfig() *Config {
	return &Config{
		Dropper: DropperConfig{
			ListenPort:     0,
			Secret:         "test-secret-for-unit-tests",
			SessionTTL:     DefaultSessionTTL,
			RateLimitLogin: DefaultRateLimitLogin,
			RootDir:        "/tmp",
			MaxUploadBytes: DefaultMaxUploadBytes,
			AuditLogPath:   DefaultAuditLogPath,
			Logging: LoggingConfig{
				Level:  DefaultLogLevel,
				Format: DefaultLogFormat,
				Output: DefaultLogOutput,
			},
		},
	}
}

func testLogger() *slog.Logger {
	return NewLogger(LoggingConfig{
		Level:  LogLevelDebug,
		Format: LogFormatConsole,
		Output: LogOutputStdout,
	}, testVersion)
}

// initTestVersion initializes go-version for tests.
// Must be called before tests that exercise the /version endpoint.
func initTestVersion(t *testing.T) {
	t.Helper()
	version.Reset()
	err := version.Initialize(
		version.WithEmbedded([]byte(testVersionsYAML)),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		version.Reset()
	})
}

func TestServer_HealthzEndpoint(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil, nil)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + RouteHealthz)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body HealthResponse
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, HealthStatusOK, body.Status)
}

func TestServer_VersionEndpoint(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil, nil)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + RouteVersion)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	project, ok := body["project"].(map[string]any)
	require.True(t, ok, "response should have a project field")
	assert.Equal(t, "dropper-test", project["name"])
	assert.Equal(t, "0.0.1-test", project["version"])
}

func TestServer_MetricsEndpoint(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil, nil)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + RouteMetrics)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(bodyBytes), "go_goroutines"),
		"metrics should contain default Go metrics")
}

func TestServer_StaticFileServing(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig()

	testFS := fstest.MapFS{
		"static/test.css": &fstest.MapFile{
			Data: []byte("body { color: red; }"),
		},
	}

	srv, err := NewServer(cfg, testLogger(), testFS, nil)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/static/test.css")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "body { color: red; }", string(bodyBytes))
}

func TestServer_SecurityHeaders(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil, nil)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + RouteHealthz)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, ValueNoSniff, resp.Header.Get(HeaderXContentTypeOpts))
	assert.Equal(t, ValueFrameDeny, resp.Header.Get(HeaderXFrameOptions))
	assert.Equal(t, ValueCSPDefault, resp.Header.Get(HeaderCSP))
}

func TestServer_NotFound(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil, nil)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/nonexistent")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
