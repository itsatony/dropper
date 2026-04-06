package dropper

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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

func testConfig(t *testing.T) *Config {
	t.Helper()
	rootDir := t.TempDir()

	// Create test file structure for browsing tests.
	require.NoError(t, os.Mkdir(filepath.Join(rootDir, "docs"), DirPermissions))
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "hello.txt"), []byte("hello world"), FilePermissions))
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "docs", "notes.txt"), []byte("notes"), FilePermissions))

	return &Config{
		Dropper: DropperConfig{
			ListenPort:     0,
			Secret:         "test-secret-for-unit-tests",
			SessionTTL:     DefaultSessionTTL,
			RateLimitLogin: DefaultRateLimitLogin,
			RootDir:        rootDir,
			MaxUploadBytes: DefaultMaxUploadBytes,
			AuditLogPath:   filepath.Join(t.TempDir(), DefaultAuditLogPath),
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
	cfg := testConfig(t)
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
	cfg := testConfig(t)
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
	cfg := testConfig(t)
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
	cfg := testConfig(t)

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
	cfg := testConfig(t)
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
	cfg := testConfig(t)
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
	cfg := testConfig(t)
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
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := noRedirectClient()

	// Step 1: POST /login with correct secret -> 303 + session cookie.
	form := url.Values{}
	form.Set(FormFieldLoginInput, cfg.Dropper.Secret)
	resp, err := client.Post(ts.URL+RouteLogin, ContentTypeForm, strings.NewReader(form.Encode()))
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
	assert.Equal(t, RouteLogin, resp2.Header.Get(HeaderLocation))

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
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := noRedirectClient()

	// POST /logout without cookie should redirect to login.
	resp, err := client.Post(ts.URL+RouteLogout, ContentTypeForm, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, RouteLogin, resp.Header.Get(HeaderLocation))
}

func TestServer_PublicRoutesNoAuth(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
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
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := noRedirectClient()

	// Exhaust rate limit with wrong secret.
	for range cfg.Dropper.RateLimitLogin {
		form := url.Values{}
		form.Set(FormFieldLoginInput, "wrong-secret-attempt")
		resp, err := client.Post(ts.URL+RouteLogin, ContentTypeForm, strings.NewReader(form.Encode()))
		require.NoError(t, err)
		resp.Body.Close()
	}

	// Next attempt should be rate limited.
	form := url.Values{}
	form.Set(FormFieldLoginInput, "wrong-secret-attempt")
	resp, err := client.Post(ts.URL+RouteLogin, ContentTypeForm, strings.NewReader(form.Encode()))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}

func TestServer_Shutdown_StopsSessionCleanup(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	// Shutdown should not panic (stops session cleanup goroutine).
	assert.NotPanics(t, func() {
		_ = srv.Shutdown(t.Context())
	})
}

// --- Audit logger integration tests ---

func TestServer_AuditLogger_Initialized(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Shutdown(t.Context()) })

	// Audit logger should be non-nil and enabled.
	require.NotNil(t, srv.AuditLogger())

	// Audit log file should exist at the configured path.
	_, err = os.Stat(cfg.Dropper.AuditLogPath)
	assert.NoError(t, err, "audit log file should exist after server creation")
}

func TestServer_Shutdown_ClosesAuditLogger(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	// Write an audit entry before shutdown.
	srv.AuditLogger().Log(AuditEntry{
		ClientIP: "10.0.0.1",
		Action:   AuditActionUpload,
		Path:     "/test.txt",
		Success:  true,
	})

	// Shutdown should close audit logger without error.
	assert.NoError(t, srv.Shutdown(t.Context()))
}

// --- File browser integration tests ---

// loginAndGetCookie performs a login and returns the session cookie.
func loginAndGetCookie(t *testing.T, tsURL string, secret string) *http.Cookie {
	t.Helper()
	client := noRedirectClient()

	form := url.Values{}
	form.Set(FormFieldLoginInput, secret)
	resp, err := client.Post(tsURL+RouteLogin, ContentTypeForm, strings.NewReader(form.Encode()))
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusSeeOther, resp.StatusCode)

	for _, c := range resp.Cookies() {
		if c.Name == SessionCookieName {
			return c
		}
	}

	t.Fatal("login did not return session cookie")
	return nil
}

func TestServer_MainPage_AuthRequired(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := noRedirectClient()

	// GET / without session → 303 to /login.
	resp, err := client.Get(ts.URL + RouteRoot)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, RouteLogin, resp.Header.Get(HeaderLocation))
}

func TestServer_MainPage_Authenticated(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	cookie := loginAndGetCookie(t, ts.URL, cfg.Dropper.Secret)

	// GET / with valid session → 200 + file browser HTML.
	req, err := http.NewRequest(http.MethodGet, ts.URL+RouteRoot, nil)
	require.NoError(t, err)
	req.AddCookie(cookie)

	resp, err := noRedirectClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	// Should contain file listing.
	assert.Contains(t, bodyStr, "docs")
	assert.Contains(t, bodyStr, "hello.txt")
	// Should contain breadcrumbs.
	assert.Contains(t, bodyStr, BreadcrumbRootLabel)
}

func TestServer_FileBrowsing_Flow(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	cookie := loginAndGetCookie(t, ts.URL, cfg.Dropper.Secret)
	client := noRedirectClient()

	// Step 1: Browse root directory.
	req, err := http.NewRequest(http.MethodGet, ts.URL+RouteRoot, nil)
	require.NoError(t, err)
	req.AddCookie(cookie)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "docs")

	// Step 2: Navigate to subdirectory via /files (HTMX style).
	req2, err := http.NewRequest(http.MethodGet, ts.URL+RouteFiles+"?"+QueryParamPath+"=docs", nil)
	require.NoError(t, err)
	req2.AddCookie(cookie)
	req2.Header.Set(HeaderHXRequest, HXRequestTrue)

	resp2, err := client.Do(req2)
	require.NoError(t, err)
	defer resp2.Body.Close()

	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	body2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	bodyStr2 := string(body2)
	assert.Contains(t, bodyStr2, "notes.txt")
	assert.Contains(t, bodyStr2, "docs")
}

func TestServer_HTMX_Navigation(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	cookie := loginAndGetCookie(t, ts.URL, cfg.Dropper.Secret)

	// HTMX request should return partial HTML (no DOCTYPE).
	req, err := http.NewRequest(http.MethodGet, ts.URL+RouteFiles+"?"+QueryParamPath+"=.", nil)
	require.NoError(t, err)
	req.AddCookie(cookie)
	req.Header.Set(HeaderHXRequest, HXRequestTrue)

	resp, err := noRedirectClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	// Partial: no full page layout.
	assert.NotContains(t, bodyStr, "<!DOCTYPE")
	// But should contain content.
	assert.Contains(t, bodyStr, BreadcrumbRootLabel)
}

func TestServer_ReadonlyMode(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	cfg.Dropper.Readonly = true

	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	cookie := loginAndGetCookie(t, ts.URL, cfg.Dropper.Secret)

	req, err := http.NewRequest(http.MethodGet, ts.URL+RouteRoot, nil)
	require.NoError(t, err)
	req.AddCookie(cookie)

	resp, err := noRedirectClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	// Readonly: no dropzone.
	assert.NotContains(t, bodyStr, "dropzone")
	// But file listing should still be present.
	assert.Contains(t, bodyStr, "docs")
}

func TestServer_LoginRedirectAfterAuth(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := noRedirectClient()

	// Login should redirect to / (RouteRoot).
	form := url.Values{}
	form.Set(FormFieldLoginInput, cfg.Dropper.Secret)
	resp, err := client.Post(ts.URL+RouteLogin, ContentTypeForm, strings.NewReader(form.Encode()))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, RouteRoot, resp.Header.Get(HeaderLocation))
}

func TestServer_FilesRoute_AuthRequired(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := noRedirectClient()

	// GET /files without session → redirect to /login.
	resp, err := client.Get(ts.URL + RouteFiles)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, RouteLogin, resp.Header.Get(HeaderLocation))
}

func TestServer_FilesRoute_JSONAuth(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// GET /files with Accept: application/json, no session → 401 JSON.
	req, err := http.NewRequest(http.MethodGet, ts.URL+RouteFiles, nil)
	require.NoError(t, err)
	req.Header.Set(HeaderAccept, ContentTypeJSON)

	resp, err := noRedirectClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	var errBody ErrorBody
	err = json.NewDecoder(resp.Body).Decode(&errBody)
	require.NoError(t, err)
	assert.Equal(t, ErrCodeUnauthorized, errBody.Code)
}

// --- File operation integration tests ---

// serverMultipartUpload creates a multipart upload request and sends it to the server.
func serverMultipartUpload(t *testing.T, tsURL string, cookie *http.Cookie, path string, files map[string][]byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for name, content := range files {
		part, err := writer.CreateFormFile(FormFieldFile, name)
		require.NoError(t, err)
		_, err = part.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	reqURL := tsURL + RouteFilesUpload + "?" + QueryParamPath + "=" + url.QueryEscape(path)
	req, err := http.NewRequest(http.MethodPost, reqURL, &buf)
	require.NoError(t, err)
	req.Header.Set(HeaderContentType, writer.FormDataContentType())
	req.AddCookie(cookie)

	resp, err := noRedirectClient().Do(req)
	require.NoError(t, err)
	return resp
}

func TestServer_FullUploadDownloadFlow(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()
	t.Cleanup(func() { _ = srv.Shutdown(t.Context()) })

	cookie := loginAndGetCookie(t, ts.URL, cfg.Dropper.Secret)

	// Step 1: Upload a file.
	uploadContent := []byte("integration test content")
	resp := serverMultipartUpload(t, ts.URL, cookie, ".", map[string][]byte{
		"integration.txt": uploadContent,
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var uploadResp UploadResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&uploadResp))
	require.Equal(t, 1, uploadResp.Uploaded)
	require.Len(t, uploadResp.Results, 1)
	finalName := uploadResp.Results[0].FinalName

	// Step 2: List files via JSON — verify uploaded file appears.
	listReq, err := http.NewRequest(http.MethodGet,
		ts.URL+RouteFiles+"?"+QueryParamPath+"=.", nil)
	require.NoError(t, err)
	listReq.AddCookie(cookie)
	listReq.Header.Set(HeaderAccept, ContentTypeJSON)

	listResp, err := noRedirectClient().Do(listReq)
	require.NoError(t, err)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var entries []FileEntry
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&entries))

	found := false
	for _, e := range entries {
		if e.Name == finalName {
			found = true
			break
		}
	}
	assert.True(t, found, "uploaded file %q should appear in file listing", finalName)

	// Step 3: Download the file — verify content matches.
	dlReq, err := http.NewRequest(http.MethodGet,
		ts.URL+RouteFilesDownload+"?"+QueryParamPath+"="+url.QueryEscape(finalName), nil)
	require.NoError(t, err)
	dlReq.AddCookie(cookie)

	dlResp, err := noRedirectClient().Do(dlReq)
	require.NoError(t, err)
	defer dlResp.Body.Close()
	require.Equal(t, http.StatusOK, dlResp.StatusCode)

	dlBody, err := io.ReadAll(dlResp.Body)
	require.NoError(t, err)
	assert.Equal(t, string(uploadContent), string(dlBody))

	// Verify Content-Disposition header.
	assert.Contains(t, dlResp.Header.Get(HeaderContentDisposition), finalName)
}

func TestServer_ReadonlyMode_WritesRejected(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	cfg.Dropper.Readonly = true

	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()
	t.Cleanup(func() { _ = srv.Shutdown(t.Context()) })

	cookie := loginAndGetCookie(t, ts.URL, cfg.Dropper.Secret)

	// Upload should be rejected.
	resp := serverMultipartUpload(t, ts.URL, cookie, ".", map[string][]byte{
		"test.txt": []byte("data"),
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	// Mkdir should be rejected.
	mkdirReq, err := http.NewRequest(http.MethodPost,
		ts.URL+RouteFilesMkdir+"?"+QueryParamPath+"=.&"+QueryParamName+"=testdir", nil)
	require.NoError(t, err)
	mkdirReq.AddCookie(cookie)

	mkdirResp, err := noRedirectClient().Do(mkdirReq)
	require.NoError(t, err)
	defer mkdirResp.Body.Close()
	assert.Equal(t, http.StatusForbidden, mkdirResp.StatusCode)
}

func TestServer_Upload_AuthRequired(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := noRedirectClient()

	// POST /files/upload without session → redirect to /login.
	resp, err := client.Post(ts.URL+RouteFilesUpload, ContentTypeForm, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, RouteLogin, resp.Header.Get(HeaderLocation))
}

func TestServer_Download_AuthRequired(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := noRedirectClient()

	// GET /files/download without session → redirect to /login.
	resp, err := client.Get(ts.URL + RouteFilesDownload + "?" + QueryParamPath + "=hello.txt")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, RouteLogin, resp.Header.Get(HeaderLocation))
}

func TestServer_Mkdir_AuthRequired(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := noRedirectClient()

	// POST /files/mkdir without session → redirect to /login.
	resp, err := client.Post(ts.URL+RouteFilesMkdir+"?"+QueryParamName+"=test", ContentTypeForm, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Equal(t, RouteLogin, resp.Header.Get(HeaderLocation))
}

func TestServer_AuditLog_RecordsOperations(t *testing.T) {
	initTestVersion(t)
	cfg := testConfig(t)
	srv, err := NewServer(cfg, testLogger(), nil, testTemplateFS())
	require.NoError(t, err)

	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	cookie := loginAndGetCookie(t, ts.URL, cfg.Dropper.Secret)

	// Upload a file.
	resp := serverMultipartUpload(t, ts.URL, cookie, ".", map[string][]byte{
		"audit-int.txt": []byte("audit data"),
	})
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Download the file.
	dlReq, err := http.NewRequest(http.MethodGet,
		ts.URL+RouteFilesDownload+"?"+QueryParamPath+"=audit-int.txt", nil)
	require.NoError(t, err)
	dlReq.AddCookie(cookie)
	dlResp, err := noRedirectClient().Do(dlReq)
	require.NoError(t, err)
	dlResp.Body.Close()

	// Mkdir.
	mkdirReq, err := http.NewRequest(http.MethodPost,
		ts.URL+RouteFilesMkdir+"?"+QueryParamPath+"=.&"+QueryParamName+"=auditsubdir", nil)
	require.NoError(t, err)
	mkdirReq.AddCookie(cookie)
	mkdirResp, err := noRedirectClient().Do(mkdirReq)
	require.NoError(t, err)
	mkdirResp.Body.Close()

	// Shutdown to flush audit log.
	require.NoError(t, srv.Shutdown(t.Context()))

	// Read the audit log.
	data, err := os.ReadFile(cfg.Dropper.AuditLogPath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.GreaterOrEqual(t, len(lines), 3, "should have at least 3 audit entries (upload + download + mkdir)")

	// Parse entries and verify actions.
	actions := make(map[string]bool)
	for _, line := range lines {
		var entry AuditEntry
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		actions[entry.Action] = true
	}

	assert.True(t, actions[AuditActionUpload], "audit log should contain upload action")
	assert.True(t, actions[AuditActionDownload], "audit log should contain download action")
	assert.True(t, actions[AuditActionMkdir], "audit log should contain mkdir action")
}
