package dropper

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- NewTemplateSet tests ---

func TestNewTemplateSet_ParsesAllTemplates(t *testing.T) {
	ts, err := NewTemplateSet(testTemplateFS())
	require.NoError(t, err)
	require.NotNil(t, ts)

	// Both page template sets must exist.
	assert.Contains(t, ts.pages, PageLogin)
	assert.Contains(t, ts.pages, PageMain)
}

func TestNewTemplateSet_NilFS(t *testing.T) {
	ts, err := NewTemplateSet(nil)
	assert.Error(t, err)
	assert.Nil(t, ts)
	assert.Contains(t, err.Error(), ErrMsgTemplateSet)
}

func TestNewTemplateSet_MissingLayout(t *testing.T) {
	incomplete := fstest.MapFS{
		"templates/login.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}login{{end}}`),
		},
	}
	ts, err := NewTemplateSet(incomplete)
	assert.Error(t, err)
	assert.Nil(t, ts)
}

func TestNewTemplateSet_MissingPartial(t *testing.T) {
	// Layout exists but a required partial is missing.
	incomplete := fstest.MapFS{
		"templates/layout.html": &fstest.MapFile{
			Data: []byte(`<!DOCTYPE html><html><body>{{block "content" .}}{{end}}</body></html>`),
		},
		"templates/login.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}login{{end}}`),
		},
	}
	ts, err := NewTemplateSet(incomplete)
	assert.Error(t, err)
	assert.Nil(t, ts)
}

// --- RenderPage tests ---

func TestRenderPage_Login(t *testing.T) {
	ts := testTemplateSet(t)

	rec := httptest.NewRecorder()
	err := ts.RenderPage(rec, PageLogin, http.StatusOK, loginData{})
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get(HeaderContentType), ContentTypeHTML)

	body := rec.Body.String()
	assert.Contains(t, body, "<form")
	assert.Contains(t, body, `name="secret"`)
}

func TestRenderPage_LoginWithError(t *testing.T) {
	ts := testTemplateSet(t)

	rec := httptest.NewRecorder()
	err := ts.RenderPage(rec, PageLogin, http.StatusUnauthorized, loginData{Error: ErrMsgInvalidCredential})
	require.NoError(t, err)

	body := rec.Body.String()
	assert.Contains(t, body, ErrMsgInvalidCredential)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRenderPage_Main(t *testing.T) {
	ts := testTemplateSet(t)

	data := PageData{
		CurrentPath: DefaultBrowsePath,
		Entries: []FileEntry{
			{Name: "docs", IsDir: true, Size: 0, FormattedSize: "0 B", ModTime: time.Now()},
			{Name: "readme.txt", IsDir: false, Size: 1024, FormattedSize: "1.0 KB", ModTime: time.Now()},
		},
		Breadcrumbs: BuildBreadcrumbs(DefaultBrowsePath),
		SortBy:      DefaultSortField,
		SortOrder:   DefaultSortOrder,
		Readonly:    false,
	}

	rec := httptest.NewRecorder()
	err := ts.RenderPage(rec, PageMain, http.StatusOK, data)
	require.NoError(t, err)

	body := rec.Body.String()
	assert.Contains(t, body, "docs")
	assert.Contains(t, body, "readme.txt")
	assert.Contains(t, body, "breadcrumbs")
	assert.Contains(t, body, "dropzone")
}

func TestRenderPage_MainReadonly(t *testing.T) {
	ts := testTemplateSet(t)

	data := PageData{
		CurrentPath: DefaultBrowsePath,
		Breadcrumbs: BuildBreadcrumbs(DefaultBrowsePath),
		SortBy:      DefaultSortField,
		SortOrder:   DefaultSortOrder,
		Readonly:    true,
	}

	rec := httptest.NewRecorder()
	err := ts.RenderPage(rec, PageMain, http.StatusOK, data)
	require.NoError(t, err)

	body := rec.Body.String()
	// Dropzone should NOT be rendered in readonly mode.
	assert.NotContains(t, body, "dropzone")
}

func TestRenderPage_UnknownPage(t *testing.T) {
	ts := testTemplateSet(t)

	rec := httptest.NewRecorder()
	err := ts.RenderPage(rec, "nonexistent", http.StatusOK, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), ErrMsgTemplateRender)
}

// --- RenderPartial tests ---

func TestRenderPartial_Breadcrumbs(t *testing.T) {
	ts := testTemplateSet(t)

	data := PageData{
		CurrentPath: "docs/2026",
		Breadcrumbs: BuildBreadcrumbs("docs/2026"),
	}

	rec := httptest.NewRecorder()
	err := ts.RenderPartial(rec, PageMain, BlockBreadcrumbs, data)
	require.NoError(t, err)

	body := rec.Body.String()
	assert.Contains(t, body, BreadcrumbRootLabel)
	assert.Contains(t, body, "docs")
	assert.Contains(t, body, "2026")
}

func TestRenderPartial_Filelist(t *testing.T) {
	ts := testTemplateSet(t)

	data := PageData{
		Entries: []FileEntry{
			{Name: "file.txt", IsDir: false, Size: 512, FormattedSize: "512 B", ModTime: time.Now()},
		},
	}

	rec := httptest.NewRecorder()
	err := ts.RenderPartial(rec, PageMain, BlockFilelist, data)
	require.NoError(t, err)

	body := rec.Body.String()
	assert.Contains(t, body, "file.txt")
	assert.Contains(t, body, "512 B")
}

func TestRenderPartial_FilelistEmpty(t *testing.T) {
	ts := testTemplateSet(t)

	data := PageData{Entries: nil}

	rec := httptest.NewRecorder()
	err := ts.RenderPartial(rec, PageMain, BlockFilelist, data)
	require.NoError(t, err)

	body := rec.Body.String()
	assert.Contains(t, body, "Empty")
}

func TestRenderPartial_UnknownPage(t *testing.T) {
	ts := testTemplateSet(t)

	rec := httptest.NewRecorder()
	err := ts.RenderPartial(rec, "nonexistent", BlockFilelist, nil)
	assert.Error(t, err)
}

// --- BuildBreadcrumbs tests ---

func TestBuildBreadcrumbs(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []BreadcrumbSegment
	}{
		{
			"root path",
			DefaultBrowsePath,
			[]BreadcrumbSegment{{Label: BreadcrumbRootLabel, Path: DefaultBrowsePath}},
		},
		{
			"empty path",
			"",
			[]BreadcrumbSegment{{Label: BreadcrumbRootLabel, Path: DefaultBrowsePath}},
		},
		{
			"single dir",
			"docs",
			[]BreadcrumbSegment{
				{Label: BreadcrumbRootLabel, Path: DefaultBrowsePath},
				{Label: "docs", Path: "docs"},
			},
		},
		{
			"nested path",
			"docs/2026/q1",
			[]BreadcrumbSegment{
				{Label: BreadcrumbRootLabel, Path: DefaultBrowsePath},
				{Label: "docs", Path: "docs"},
				{Label: "2026", Path: "docs/2026"},
				{Label: "q1", Path: "docs/2026/q1"},
			},
		},
		{
			"path with dot",
			"./docs",
			[]BreadcrumbSegment{
				{Label: BreadcrumbRootLabel, Path: DefaultBrowsePath},
				{Label: "docs", Path: "docs"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildBreadcrumbs(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- IsHTMXRequest tests ---

func TestIsHTMXRequest(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected bool
	}{
		{"htmx request", HXRequestTrue, true},
		{"no header", "", false},
		{"wrong value", "false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set(HeaderHXRequest, tt.header)
			}
			assert.Equal(t, tt.expected, IsHTMXRequest(req))
		})
	}
}

// --- PageData.ChildPath tests ---

func TestPageData_ChildPath(t *testing.T) {
	tests := []struct {
		name        string
		currentPath string
		child       string
		expected    string
	}{
		{"root child", DefaultBrowsePath, "docs", "docs"},
		{"empty current", "", "docs", "docs"},
		{"nested child", "docs", "2026", "docs/2026"},
		{"deep nested", "docs/2026", "q1", "docs/2026/q1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := PageData{CurrentPath: tt.currentPath}
			assert.Equal(t, tt.expected, data.ChildPath(tt.child))
		})
	}
}

// --- Template function tests ---

func TestFormatModTime(t *testing.T) {
	ts := time.Date(2026, 4, 6, 14, 30, 0, 0, time.UTC)
	result := formatModTime(ts)
	assert.Equal(t, "2026-04-06 14:30", result)
}

// --- Template urlquery function test ---

func TestTemplateFuncMap_URLQuery(t *testing.T) {
	funcMap := templateFuncMap()
	urlqueryFn, ok := funcMap["urlquery"]
	require.True(t, ok, "urlquery must be in template func map")

	// Cast and test.
	fn, ok := urlqueryFn.(func(string) string)
	require.True(t, ok, "urlquery must be func(string) string")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple path", "docs", "docs"},
		{"path with spaces", "my docs", "my+docs"},
		{"path with ampersand", "a&b", "a%26b"},
		{"path with slash", "docs/2026", "docs%2F2026"},
		{"dot path", ".", "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, fn(tt.input))
		})
	}
}
