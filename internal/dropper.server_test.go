package dropper

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
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
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
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
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
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

	srv, err := NewServer(cfg, testLogger(), testFS, testTemplateFS())
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
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
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
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/nonexistent")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// --- Auth integration tests ---

// noRedirectClient returns an HTTP client that does NOT follow redirects.
func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func TestServer_LoginPage(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	resp, err := http.Get(ts.URL + RouteLogin)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "<form")
	assert.Contains(t, string(body), `name="secret"`)
}

func TestServer_LoginLogoutFlow(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := noRedirectClient()

	// Step 1: POST /login with correct secret -> 303 + session cookie.
	form := url.Values{}
	form.Set(FormFieldLoginInput, cfg.Dropper.Secret)
	resp, err := client.Post(ts.URL+RouteLogin, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)

	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == SessionCookieName {
			sessionCookie = c
			break
		}
	}
	require.NotNil(t, sessionCookie, "login must set session cookie")

	// Step 2: POST /logout with session cookie -> 303 + cleared cookie.
	logoutReq, err := http.NewRequest(http.MethodPost, ts.URL+RouteLogout, nil)
	require.NoError(t, err)
	logoutReq.AddCookie(sessionCookie)

	resp2, err := client.Do(logoutReq)
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusSeeOther, resp2.StatusCode)
	assert.Equal(t, RouteLogin, resp2.Header.Get("Location"))

	// Step 3: POST /logout with the old cookie -> still redirects (session deleted).
	logoutReq2, err := http.NewRequest(http.MethodPost, ts.URL+RouteLogout, nil)
	require.NoError(t, err)
	logoutReq2.AddCookie(sessionCookie)

	resp3, err := client.Do(logoutReq2)
	require.NoError(t, err)
	defer resp3.Body.Close()

	// Session middleware should redirect to login since session was deleted.
	assert.Equal(t, http.StatusSeeOther, resp3.StatusCode)
}

func TestServer_UnauthenticatedRedirect(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := noRedirectClient()

	// POST /logout without cookie should redirect to login.
	resp, err := client.Post(ts.URL+RouteLogout, "application/x-www-form-urlencoded", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, RouteLogin, resp.Header.Get("Location"))
}

func TestServer_PublicRoutesNoAuth(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// All these routes should be accessible without a session cookie.
	publicRoutes := []string{RouteHealthz, RouteVersion, RouteMetrics, RouteLogin}

	for _, route := range publicRoutes {
		t.Run(route, func(t *testing.T) {
			resp, err := http.Get(ts.URL + route)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode, "route %s should be public", route)
		})
	}
}

func TestServer_LoginRateLimiting(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := noRedirectClient()

	// Exhaust rate limit with wrong secret.
	for range cfg.Dropper.RateLimitLogin {
		form := url.Values{}
		form.Set(FormFieldLoginInput, "wrong-secret-attempt")
		resp, err := client.Post(ts.URL+RouteLogin, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		require.NoError(t, err)
		resp.Body.Close()
	}

	// Next attempt should be rate limited.
	form := url.Values{}
	form.Set(FormFieldLoginInput, "wrong-secret-attempt")
	resp, err := client.Post(ts.URL+RouteLogin, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}

func TestServer_Shutdown_StopsSessionCleanup(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig()
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	// Shutdown should not panic (stops session cleanup goroutine).
	assert.NotPanics(t, func() {
		_ = srv.Shutdown(t.Context())
	})
}
