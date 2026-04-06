package dropper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test helpers ---

// testDropperConfig returns a DropperConfig with a real temp directory as RootDir.
func testDropperConfig(t *testing.T) *DropperConfig {
	t.Helper()
	rootDir := t.TempDir()

	// Create some test files and directories.
	require.NoError(t, os.Mkdir(filepath.Join(rootDir, "docs"), DirPermissions))
	require.NoError(t, os.Mkdir(filepath.Join(rootDir, "images"), DirPermissions))
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "readme.txt"), []byte("hello"), FilePermissions))
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "docs", "notes.txt"), []byte("notes"), FilePermissions))

	return &DropperConfig{
		RootDir:  rootDir,
		Readonly: false,
	}
}

// testDropperConfigReadonly returns a readonly DropperConfig.
func testDropperConfigReadonly(t *testing.T) *DropperConfig {
	t.Helper()
	cfg := testDropperConfig(t)
	cfg.Readonly = true
	return cfg
}

// --- HandleMainPage tests ---

func TestHandleMainPage_Authenticated(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleMainPage(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteRoot, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get(HeaderContentType), ContentTypeHTML)

	body := rec.Body.String()
	// Should contain file listing.
	assert.Contains(t, body, "docs")
	assert.Contains(t, body, "images")
	assert.Contains(t, body, "readme.txt")
	// Should contain breadcrumbs.
	assert.Contains(t, body, BreadcrumbRootLabel)
	// Should contain dropzone (not readonly).
	assert.Contains(t, body, "dropzone")
}

func TestHandleMainPage_WithSubdir(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleMainPage(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteRoot+"?"+QueryParamPath+"=docs", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "notes.txt")
	// Breadcrumbs should include docs.
	assert.Contains(t, body, "docs")
	assert.Contains(t, body, BreadcrumbRootLabel)
}

func TestHandleMainPage_ReadonlyMode(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfigReadonly(t)
	handler := HandleMainPage(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteRoot, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	// File listing should still be present.
	assert.Contains(t, body, "docs")
	// Dropzone should NOT be present in readonly mode.
	assert.NotContains(t, body, "dropzone")
}

func TestHandleMainPage_InvalidPath_JSON(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleMainPage(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteRoot+"?"+QueryParamPath+"=../../etc", nil)
	req.Header.Set(HeaderAccept, ContentTypeJSON)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Path traversal should be denied.
	assert.Equal(t, http.StatusForbidden, rec.Code)

	var errBody ErrorBody
	err := json.NewDecoder(rec.Body).Decode(&errBody)
	require.NoError(t, err)
	assert.Equal(t, ErrCodeForbidden, errBody.Code)
	// Must not leak path information.
	assert.NotContains(t, errBody.Message, "etc")
}

func TestHandleMainPage_InvalidPath_HTML(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleMainPage(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteRoot+"?"+QueryParamPath+"=../../etc", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should render a page with error state and 403 status.
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Header().Get(HeaderContentType), ContentTypeHTML)
}

// --- HandleListFiles tests ---

func TestHandleListFiles_HTMXRequest(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleListFiles(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFiles+"?"+QueryParamPath+"="+DefaultBrowsePath, nil)
	req.Header.Set(HeaderHXRequest, HXRequestTrue)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get(HeaderContentType), ContentTypeHTML)

	body := rec.Body.String()
	// Should contain file entries as partials.
	assert.Contains(t, body, "docs")
	assert.Contains(t, body, "readme.txt")
	// Should contain breadcrumbs.
	assert.Contains(t, body, BreadcrumbRootLabel)
	// Should NOT contain full page layout.
	assert.NotContains(t, body, "<!DOCTYPE")
}

func TestHandleListFiles_JSONRequest(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleListFiles(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFiles+"?"+QueryParamPath+"="+DefaultBrowsePath, nil)
	req.Header.Set(HeaderAccept, ContentTypeJSON)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get(HeaderContentType), ContentTypeJSON)

	var entries []FileEntry
	err := json.NewDecoder(rec.Body).Decode(&entries)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(entries), 3) // docs, images, readme.txt
}

func TestHandleListFiles_BrowserRedirect(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleListFiles(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFiles+"?"+QueryParamPath+"=docs", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should redirect to /?path=docs.
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, rec.Header().Get(HeaderLocation), RouteRoot)
	assert.Contains(t, rec.Header().Get(HeaderLocation), "docs")
}

func TestHandleListFiles_SortByName(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleListFiles(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFiles+"?"+QueryParamPath+"=.&"+QueryParamSortBy+"=name&"+QueryParamSortOrder+"=asc", nil)
	req.Header.Set(HeaderAccept, ContentTypeJSON)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var entries []FileEntry
	err := json.NewDecoder(rec.Body).Decode(&entries)
	require.NoError(t, err)

	// Directories first, then files.
	require.GreaterOrEqual(t, len(entries), 3)
	assert.True(t, entries[0].IsDir, "first entries should be directories")
}

func TestHandleListFiles_SortBySize(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleListFiles(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFiles+"?"+QueryParamPath+"=.&"+QueryParamSortBy+"=size&"+QueryParamSortOrder+"=desc", nil)
	req.Header.Set(HeaderAccept, ContentTypeJSON)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var entries []FileEntry
	err := json.NewDecoder(rec.Body).Decode(&entries)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(entries), 1)
}

func TestHandleListFiles_InvalidPath(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleListFiles(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFiles+"?"+QueryParamPath+"=../../etc", nil)
	req.Header.Set(HeaderAccept, ContentTypeJSON)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)

	var errBody ErrorBody
	err := json.NewDecoder(rec.Body).Decode(&errBody)
	require.NoError(t, err)
	assert.Equal(t, ErrCodeForbidden, errBody.Code)
	// Must not leak path info.
	assert.NotContains(t, errBody.Message, "etc")
}

func TestHandleListFiles_NonexistentDir(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleListFiles(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFiles+"?"+QueryParamPath+"=nonexistent", nil)
	req.Header.Set(HeaderAccept, ContentTypeJSON)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleListFiles_InvalidSortParams(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleListFiles(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFiles+"?"+QueryParamPath+"=.&"+QueryParamSortBy+"=invalid&"+QueryParamSortOrder+"=invalid", nil)
	req.Header.Set(HeaderAccept, ContentTypeJSON)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var entries []FileEntry
	err := json.NewDecoder(rec.Body).Decode(&entries)
	require.NoError(t, err)

	// Invalid sort falls back to default (name asc) — directories first.
	require.GreaterOrEqual(t, len(entries), 3)
	assert.True(t, entries[0].IsDir, "directories should come first with default sort")
}

func TestHandleListFiles_HTMXErrorPath(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleListFiles(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFiles+"?"+QueryParamPath+"=../../etc", nil)
	req.Header.Set(HeaderHXRequest, HXRequestTrue)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// HTMX error should return 403 with HTML content, not JSON.
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Header().Get(HeaderContentType), ContentTypeHTML)

	body := rec.Body.String()
	// Should contain breadcrumbs (reset to root) and filelist partials.
	assert.Contains(t, body, BreadcrumbRootLabel)
	// Must not contain path info.
	assert.NotContains(t, body, "etc")
}

func TestHandleListFiles_BrowserErrorRedirect(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleListFiles(ts, cfg, authTestLogger())

	// Browser request (no HX-Request, no Accept: JSON) with bad path.
	req := httptest.NewRequest(http.MethodGet, RouteFiles+"?"+QueryParamPath+"=../../etc", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Browser should be redirected to root.
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, rec.Header().Get(HeaderLocation), RouteRoot)
}

func TestHandleListFiles_Subdir(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleListFiles(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFiles+"?"+QueryParamPath+"=docs", nil)
	req.Header.Set(HeaderHXRequest, HXRequestTrue)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "notes.txt")
}
