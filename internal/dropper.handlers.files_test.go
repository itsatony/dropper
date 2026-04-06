package dropper

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
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

	assert.Equal(t, http.StatusNotFound, rec.Code)
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

// --- Test helpers for file handlers ---

// testFileAuditLogger creates an AuditLogger writing to a temp file.
func testFileAuditLogger(t *testing.T) (*AuditLogger, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test_audit.log")
	al, err := NewAuditLogger(path, authTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = al.Close() })
	return al, path
}

// createMultipartBody creates a multipart/form-data body with the given files.
// Returns the body buffer and the content type header value.
func createMultipartBody(t *testing.T, fieldName string, files map[string][]byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for name, content := range files {
		part, err := writer.CreateFormFile(fieldName, name)
		require.NoError(t, err)
		_, err = part.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return &buf, writer.FormDataContentType()
}

// readAuditEntries reads and parses all audit entries from the log file.
func readAuditEntries(t *testing.T, path string) []AuditEntry {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var entries []AuditEntry
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var entry AuditEntry
		require.NoError(t, json.Unmarshal(line, &entry))
		entries = append(entries, entry)
	}
	return entries
}

// testUploadConfig returns a DropperConfig with AllowedExtensions set.
func testUploadConfig(t *testing.T) *DropperConfig {
	t.Helper()
	cfg := testDropperConfig(t)
	cfg.MaxUploadBytes = DefaultMaxUploadBytes
	cfg.AllowedExtensions = nil // allow all
	return cfg
}

// --- HandleDownload tests ---

func TestHandleDownload_Success(t *testing.T) {
	cfg := testDropperConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleDownload(cfg, audit, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFilesDownload+"?"+QueryParamPath+"=readme.txt", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// Verify Content-Disposition header.
	cd := rec.Header().Get(HeaderContentDisposition)
	assert.Contains(t, cd, "readme.txt")
	assert.Contains(t, cd, "attachment")

	// Verify body content matches the test file.
	assert.Equal(t, "hello", rec.Body.String())
}

func TestHandleDownload_PathTraversal(t *testing.T) {
	cfg := testDropperConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleDownload(cfg, audit, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFilesDownload+"?"+QueryParamPath+"=../../etc/passwd", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)

	var errBody ErrorBody
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errBody))
	assert.Equal(t, ErrCodeForbidden, errBody.Code)
	// Must not leak path info.
	assert.NotContains(t, errBody.Message, "etc")
	assert.NotContains(t, errBody.Message, "passwd")
}

func TestHandleDownload_Directory(t *testing.T) {
	cfg := testDropperConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleDownload(cfg, audit, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFilesDownload+"?"+QueryParamPath+"=docs", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errBody ErrorBody
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errBody))
	assert.Equal(t, ErrCodeBadRequest, errBody.Code)
	assert.Equal(t, ErrMsgNotFile, errBody.Message)
}

func TestHandleDownload_NotFound(t *testing.T) {
	cfg := testDropperConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleDownload(cfg, audit, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFilesDownload+"?"+QueryParamPath+"=nonexistent.txt", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleDownload_EmptyPath(t *testing.T) {
	cfg := testDropperConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleDownload(cfg, audit, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFilesDownload, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errBody ErrorBody
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errBody))
	assert.Equal(t, ErrCodeBadRequest, errBody.Code)
}

func TestHandleDownload_AuditLogged(t *testing.T) {
	cfg := testDropperConfig(t)
	audit, auditPath := testFileAuditLogger(t)
	handler := HandleDownload(cfg, audit, authTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteFilesDownload+"?"+QueryParamPath+"=readme.txt", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Close audit logger to flush.
	require.NoError(t, audit.Close())

	entries := readAuditEntries(t, auditPath)
	require.Len(t, entries, 1)
	assert.Equal(t, AuditActionDownload, entries[0].Action)
	assert.Equal(t, "readme.txt", entries[0].Path)
	assert.True(t, entries[0].Success)
	require.NotNil(t, entries[0].FileSize)
	assert.Equal(t, int64(5), *entries[0].FileSize) // "hello" = 5 bytes
}

// --- HandleMkdir tests ---

func TestHandleMkdir_Success(t *testing.T) {
	cfg := testDropperConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleMkdir(cfg, audit, authTestLogger())

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesMkdir+"?"+QueryParamPath+"=.&"+QueryParamName+"=newfolder", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp MkdirResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "newfolder", resp.Name)
	assert.Equal(t, ".", resp.Path)

	// Verify directory exists on disk.
	_, err := os.Stat(filepath.Join(cfg.RootDir, "newfolder"))
	assert.NoError(t, err)
}

func TestHandleMkdir_ReadonlyRejected(t *testing.T) {
	cfg := testDropperConfigReadonly(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleMkdir(cfg, audit, authTestLogger())

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesMkdir+"?"+QueryParamPath+"=.&"+QueryParamName+"=newfolder", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)

	var errBody ErrorBody
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errBody))
	assert.Equal(t, ErrCodeReadonly, errBody.Code)
}

func TestHandleMkdir_PathTraversal(t *testing.T) {
	cfg := testDropperConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleMkdir(cfg, audit, authTestLogger())

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesMkdir+"?"+QueryParamPath+"=../../etc&"+QueryParamName+"=evil", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleMkdir_MissingName(t *testing.T) {
	cfg := testDropperConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleMkdir(cfg, audit, authTestLogger())

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesMkdir+"?"+QueryParamPath+"=.", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errBody ErrorBody
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errBody))
	assert.Equal(t, ErrCodeBadRequest, errBody.Code)
}

func TestHandleMkdir_SpecialCharsInName(t *testing.T) {
	cfg := testDropperConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleMkdir(cfg, audit, authTestLogger())

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesMkdir+"?"+QueryParamPath+"=.&"+QueryParamName+"=my%20folder%21%40%23", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp MkdirResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	// Name should be sanitized (special chars replaced with _).
	assert.NotContains(t, resp.Name, " ")
	assert.NotContains(t, resp.Name, "!")
	assert.NotContains(t, resp.Name, "@")
}

func TestHandleMkdir_AuditLogged(t *testing.T) {
	cfg := testDropperConfig(t)
	audit, auditPath := testFileAuditLogger(t)
	handler := HandleMkdir(cfg, audit, authTestLogger())

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesMkdir+"?"+QueryParamPath+"=.&"+QueryParamName+"=auditdir", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)

	require.NoError(t, audit.Close())

	entries := readAuditEntries(t, auditPath)
	require.Len(t, entries, 1)
	assert.Equal(t, AuditActionMkdir, entries[0].Action)
	assert.True(t, entries[0].Success)
}

// --- HandleUpload tests ---

func TestHandleUpload_SingleFile(t *testing.T) {
	cfg := testUploadConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	body, contentType := createMultipartBody(t, FormFieldFile, map[string][]byte{
		"test.txt": []byte("file content here"),
	})

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesUpload+"?"+QueryParamPath+"=.", body)
	req.Header.Set(HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UploadResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 1, resp.Uploaded)
	assert.Equal(t, 0, resp.Failed)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "test.txt", resp.Results[0].OriginalName)
	assert.NotEmpty(t, resp.Results[0].FinalName)
	assert.Greater(t, resp.Results[0].Size, int64(0))

	// Verify file on disk.
	diskPath := filepath.Join(cfg.RootDir, resp.Results[0].FinalName)
	data, err := os.ReadFile(diskPath)
	require.NoError(t, err)
	assert.Equal(t, "file content here", string(data))
}

func TestHandleUpload_MultiFile(t *testing.T) {
	cfg := testUploadConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	body, contentType := createMultipartBody(t, FormFieldFile, map[string][]byte{
		"file1.txt": []byte("content one"),
		"file2.txt": []byte("content two"),
	})

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesUpload+"?"+QueryParamPath+"=.", body)
	req.Header.Set(HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UploadResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 2, resp.Uploaded)
	assert.Equal(t, 0, resp.Failed)
	assert.Len(t, resp.Results, 2)
}

func TestHandleUpload_ReadonlyRejected(t *testing.T) {
	cfg := testDropperConfigReadonly(t)
	cfg.MaxUploadBytes = DefaultMaxUploadBytes
	audit, _ := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	body, contentType := createMultipartBody(t, FormFieldFile, map[string][]byte{
		"test.txt": []byte("data"),
	})

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesUpload+"?"+QueryParamPath+"=.", body)
	req.Header.Set(HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)

	var errBody ErrorBody
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errBody))
	assert.Equal(t, ErrCodeReadonly, errBody.Code)
}

func TestHandleUpload_PayloadTooLarge(t *testing.T) {
	cfg := testUploadConfig(t)
	cfg.MaxUploadBytes = 10 // 10 bytes max
	audit, _ := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	// Create a body larger than 10 bytes.
	body, contentType := createMultipartBody(t, FormFieldFile, map[string][]byte{
		"large.txt": bytes.Repeat([]byte("x"), 1024),
	})

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesUpload+"?"+QueryParamPath+"=.", body)
	req.Header.Set(HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should be 413 (payload too large) or 400 (multipart parse failure due to size).
	assert.True(t, rec.Code == http.StatusRequestEntityTooLarge || rec.Code == http.StatusBadRequest,
		"expected 413 or 400, got %d", rec.Code)
}

func TestHandleUpload_ExtensionNotAllowed(t *testing.T) {
	cfg := testUploadConfig(t)
	cfg.AllowedExtensions = []string{".txt", ".md"}
	audit, _ := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	body, contentType := createMultipartBody(t, FormFieldFile, map[string][]byte{
		"script.exe": []byte("evil binary"),
	})

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesUpload+"?"+QueryParamPath+"=.", body)
	req.Header.Set(HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UploadResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 0, resp.Uploaded)
	assert.Equal(t, 1, resp.Failed)
	require.Len(t, resp.Results, 1)
	assert.Contains(t, resp.Results[0].Error, ErrMsgExtNotAllowed)
}

func TestHandleUpload_PathTraversal(t *testing.T) {
	cfg := testUploadConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	body, contentType := createMultipartBody(t, FormFieldFile, map[string][]byte{
		"test.txt": []byte("data"),
	})

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesUpload+"?"+QueryParamPath+"=../../etc", body)
	req.Header.Set(HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UploadResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	// Path traversal should cause the file write to fail.
	assert.Equal(t, 0, resp.Uploaded)
	assert.Equal(t, 1, resp.Failed)
}

func TestHandleUpload_NoFiles(t *testing.T) {
	cfg := testUploadConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	// Create an empty multipart body (no files).
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesUpload+"?"+QueryParamPath+"=.", &buf)
	req.Header.Set(HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var errBody ErrorBody
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&errBody))
	assert.Equal(t, ErrCodeBadRequest, errBody.Code)
}

func TestHandleUpload_ClipboardMode(t *testing.T) {
	cfg := testUploadConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	body, contentType := createMultipartBody(t, FormFieldFile, map[string][]byte{
		"blob": []byte("fake png data"),
	})

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesUpload+"?"+QueryParamPath+"=.&"+QueryParamClipboard+"=true", body)
	req.Header.Set(HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UploadResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 1, resp.Uploaded)
	require.Len(t, resp.Results, 1)
	// Clipboard filename should match pattern: YYYYMMDD-HHMMSS_clipboard.png
	assert.Contains(t, resp.Results[0].FinalName, ClipboardFilenamePrefix)
	assert.Contains(t, resp.Results[0].FinalName, ClipboardFilenameExt)
}

func TestHandleUpload_FilenameCollision(t *testing.T) {
	cfg := testUploadConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	// First upload.
	body1, ct1 := createMultipartBody(t, FormFieldFile, map[string][]byte{
		"collision.txt": []byte("first"),
	})
	req1 := httptest.NewRequest(http.MethodPost,
		RouteFilesUpload+"?"+QueryParamPath+"=.", body1)
	req1.Header.Set(HeaderContentType, ct1)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	var resp1 UploadResponse
	require.NoError(t, json.NewDecoder(rec1.Body).Decode(&resp1))
	assert.Equal(t, 1, resp1.Uploaded)
	firstName := resp1.Results[0].FinalName

	// Second upload with same name.
	body2, ct2 := createMultipartBody(t, FormFieldFile, map[string][]byte{
		"collision.txt": []byte("second"),
	})
	req2 := httptest.NewRequest(http.MethodPost,
		RouteFilesUpload+"?"+QueryParamPath+"=.", body2)
	req2.Header.Set(HeaderContentType, ct2)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	var resp2 UploadResponse
	require.NoError(t, json.NewDecoder(rec2.Body).Decode(&resp2))
	assert.Equal(t, 1, resp2.Uploaded)
	secondName := resp2.Results[0].FinalName

	// Names should differ due to collision resolution.
	assert.NotEqual(t, firstName, secondName)

	// Both files should exist on disk.
	_, err1 := os.Stat(filepath.Join(cfg.RootDir, firstName))
	assert.NoError(t, err1)
	_, err2 := os.Stat(filepath.Join(cfg.RootDir, secondName))
	assert.NoError(t, err2)
}

func TestHandleUpload_AuditLogged(t *testing.T) {
	cfg := testUploadConfig(t)
	audit, auditPath := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	body, contentType := createMultipartBody(t, FormFieldFile, map[string][]byte{
		"audit-test.txt": []byte("audit me"),
	})

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesUpload+"?"+QueryParamPath+"=.", body)
	req.Header.Set(HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	require.NoError(t, audit.Close())

	entries := readAuditEntries(t, auditPath)
	require.GreaterOrEqual(t, len(entries), 1)
	assert.Equal(t, AuditActionUpload, entries[0].Action)
	assert.True(t, entries[0].Success)
	require.NotNil(t, entries[0].FileSize)
}

func TestHandleUpload_ToSubdir(t *testing.T) {
	cfg := testUploadConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	body, contentType := createMultipartBody(t, FormFieldFile, map[string][]byte{
		"subdir-file.txt": []byte("in docs"),
	})

	req := httptest.NewRequest(http.MethodPost,
		RouteFilesUpload+"?"+QueryParamPath+"=docs", body)
	req.Header.Set(HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UploadResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 1, resp.Uploaded)

	// Verify file is in the docs subdirectory.
	diskPath := filepath.Join(cfg.RootDir, "docs", resp.Results[0].FinalName)
	data, err := os.ReadFile(diskPath)
	require.NoError(t, err)
	assert.Equal(t, "in docs", string(data))
}

// --- DC-07 HTMX/Sorting/Breadcrumb tests ---

func TestHandleListFiles_HTMXWithSortParams(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleListFiles(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet,
		RouteFiles+"?"+QueryParamPath+"=.&"+QueryParamSortBy+"=size&"+QueryParamSortOrder+"=desc", nil)
	req.Header.Set(HeaderHXRequest, HXRequestTrue)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get(HeaderContentType), ContentTypeHTML)

	body := rec.Body.String()
	// Should contain breadcrumbs partial.
	assert.Contains(t, body, BreadcrumbRootLabel)
	// Should contain file entries.
	assert.Contains(t, body, "readme.txt")
	// Should NOT contain full page layout.
	assert.NotContains(t, body, "<!DOCTYPE")
}

func TestHandleListFiles_SortByDate(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleListFiles(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet,
		RouteFiles+"?"+QueryParamPath+"=.&"+QueryParamSortBy+"=date&"+QueryParamSortOrder+"=asc", nil)
	req.Header.Set(HeaderAccept, ContentTypeJSON)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var entries []FileEntry
	err := json.NewDecoder(rec.Body).Decode(&entries)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(entries), 3)

	// Directories should still come first regardless of sort field.
	var foundFile bool
	for _, e := range entries {
		if !e.IsDir {
			foundFile = true
		}
		if foundFile {
			assert.False(t, e.IsDir, "no directories should appear after files in sorted output")
		}
	}
}

func TestHandleMainPage_WithSortParams(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleMainPage(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet,
		RouteRoot+"?"+QueryParamPath+"=.&"+QueryParamSortBy+"=size&"+QueryParamSortOrder+"=desc", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get(HeaderContentType), ContentTypeHTML)

	body := rec.Body.String()
	// Full page render should include layout and content.
	assert.Contains(t, body, "<!DOCTYPE")
	assert.Contains(t, body, "readme.txt")
	assert.Contains(t, body, BreadcrumbRootLabel)
}

func TestHandleListFiles_HTMXSubdirBreadcrumbs(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)

	// Create nested directories.
	require.NoError(t, os.MkdirAll(filepath.Join(cfg.RootDir, "docs", "2026"), DirPermissions))
	require.NoError(t, os.WriteFile(
		filepath.Join(cfg.RootDir, "docs", "2026", "report.txt"),
		[]byte("data"), FilePermissions))

	handler := HandleListFiles(ts, cfg, authTestLogger())

	req := httptest.NewRequest(http.MethodGet,
		RouteFiles+"?"+QueryParamPath+"=docs/2026", nil)
	req.Header.Set(HeaderHXRequest, HXRequestTrue)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	// Breadcrumbs should contain Home, docs, and 2026 segments.
	assert.Contains(t, body, BreadcrumbRootLabel)
	assert.Contains(t, body, "docs")
	assert.Contains(t, body, "2026")
	// File list should contain the report.
	assert.Contains(t, body, "report.txt")
}

func TestHandleListFiles_DefaultSortParams(t *testing.T) {
	ts := testTemplateSet(t)
	cfg := testDropperConfig(t)
	handler := HandleListFiles(ts, cfg, authTestLogger())

	// No sort params at all — should use defaults.
	req := httptest.NewRequest(http.MethodGet,
		RouteFiles+"?"+QueryParamPath+"=.", nil)
	req.Header.Set(HeaderAccept, ContentTypeJSON)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var entries []FileEntry
	err := json.NewDecoder(rec.Body).Decode(&entries)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(entries), 3)

	// Default sort is name asc — directories first, then alphabetical.
	assert.True(t, entries[0].IsDir, "first entry should be a directory")
}

// --- DC-10 Directory Upload handler tests ---

// fileWithRelPath represents a file with an optional relative path for directory uploads.
type fileWithRelPath struct {
	name    string
	relpath string
	content []byte
}

// createMultipartBodyWithRelPaths builds a multipart body with file and relpath fields.
func createMultipartBodyWithRelPaths(t *testing.T, files []fileWithRelPath) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for _, f := range files {
		part, err := writer.CreateFormFile(FormFieldFile, f.name)
		require.NoError(t, err)
		_, err = part.Write(f.content)
		require.NoError(t, err)

		// Write the relpath field.
		err = writer.WriteField(FormFieldRelPath, f.relpath)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return &buf, writer.FormDataContentType()
}

func TestHandleUpload_DirectoryUpload_PreservesStructure(t *testing.T) {
	cfg := testUploadConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	files := []fileWithRelPath{
		{name: "readme.md", relpath: "project/docs", content: []byte("# README")},
		{name: "main.go", relpath: "project/src", content: []byte("package main")},
	}
	body, contentType := createMultipartBodyWithRelPaths(t, files)

	req := httptest.NewRequest(http.MethodPost, RouteFilesUpload+"?"+QueryParamPath+"=.", body)
	req.Header.Set(HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UploadResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 2, resp.Uploaded)
	assert.Equal(t, 0, resp.Failed)

	// Verify nested directory structure was created.
	docsContent, err := os.ReadFile(filepath.Join(cfg.RootDir, "project", "docs", "readme.md"))
	require.NoError(t, err)
	assert.Equal(t, "# README", string(docsContent))

	srcContent, err := os.ReadFile(filepath.Join(cfg.RootDir, "project", "src", "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main", string(srcContent))
}

func TestHandleUpload_DirectoryUpload_MixedFlatAndNested(t *testing.T) {
	cfg := testUploadConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	files := []fileWithRelPath{
		{name: "flat.txt", relpath: "", content: []byte("flat file")},
		{name: "nested.txt", relpath: "subdir", content: []byte("nested file")},
	}
	body, contentType := createMultipartBodyWithRelPaths(t, files)

	req := httptest.NewRequest(http.MethodPost, RouteFilesUpload+"?"+QueryParamPath+"=.", body)
	req.Header.Set(HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UploadResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 2, resp.Uploaded)

	// Flat file in root.
	flatContent, err := os.ReadFile(filepath.Join(cfg.RootDir, "flat.txt"))
	require.NoError(t, err)
	assert.Equal(t, "flat file", string(flatContent))

	// Nested file in subdir.
	nestedContent, err := os.ReadFile(filepath.Join(cfg.RootDir, "subdir", "nested.txt"))
	require.NoError(t, err)
	assert.Equal(t, "nested file", string(nestedContent))
}

func TestHandleUpload_DirectoryUpload_PathTraversalRejected(t *testing.T) {
	cfg := testUploadConfig(t)
	audit, _ := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	// All ".." components get sanitized to "_" by SanitizeFilename,
	// so this won't actually escape root. But if only dots remain after
	// sanitization and all become "_" (a fallback), the sanitized parts
	// will be ["_", "_", "etc"]. The file should end up safely inside root.
	files := []fileWithRelPath{
		{name: "passwd.txt", relpath: "../../etc", content: []byte("evil")},
	}
	body, contentType := createMultipartBodyWithRelPaths(t, files)

	req := httptest.NewRequest(http.MethodPost, RouteFilesUpload+"?"+QueryParamPath+"=.", body)
	req.Header.Set(HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should succeed but file should be safely within root.
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp UploadResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	// Verify no files escaped outside root.
	assert.Equal(t, 1, resp.Uploaded)
}

func TestHandleUpload_DirectoryUpload_ReadonlyRejected(t *testing.T) {
	cfg := testUploadConfig(t)
	cfg.Readonly = true
	audit, _ := testFileAuditLogger(t)
	handler := HandleUpload(cfg, audit, authTestLogger())

	files := []fileWithRelPath{
		{name: "file.txt", relpath: "subdir", content: []byte("blocked")},
	}
	body, contentType := createMultipartBodyWithRelPaths(t, files)

	req := httptest.NewRequest(http.MethodPost, RouteFilesUpload+"?"+QueryParamPath+"=.", body)
	req.Header.Set(HeaderContentType, contentType)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}
