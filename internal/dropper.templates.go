package dropper

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// BreadcrumbSegment represents one segment in the breadcrumb navigation.
type BreadcrumbSegment struct {
	Label string
	Path  string
}

// PageData carries all data needed to render a full page template.
type PageData struct {
	CurrentPath string
	Entries     []FileEntry
	Breadcrumbs []BreadcrumbSegment
	SortBy      string
	SortOrder   string
	Readonly    bool
	Error       string
	DiskUsage   *DiskUsageInfo
}

// DiskUsageData is the data passed to the diskusage partial when rendered
// via HTMX from the /healthz endpoint.
type DiskUsageData struct {
	DiskUsage *DiskUsageInfo
}

// ChildPath returns the path to a child entry within the current directory.
// Used by templates to build navigation links.
func (p PageData) ChildPath(name string) string {
	if p.CurrentPath == DefaultBrowsePath || p.CurrentPath == "" {
		return name
	}
	return path.Join(p.CurrentPath, name)
}

// loginData is the data passed to the login template.
type loginData struct {
	Error string
}

// TemplateSet holds pre-compiled page template sets.
// Each page gets its own clone of the base templates (layout + partials + components)
// with the page-specific template parsed into it. This avoids block name collisions
// between pages that both define {{define "content"}}.
//
// Thread-safe: Go templates are safe for concurrent ExecuteTemplate after Parse.
type TemplateSet struct {
	pages map[string]*template.Template
}

// templateFuncMap returns the function map available to all templates.
func templateFuncMap() template.FuncMap {
	return template.FuncMap{
		"formatSize":     FormatFileSize,
		"formatTime":     formatModTime,
		"formatDiskSize": FormatDiskSizeFloat,
		"diskPercent":    FormatDiskPercent,
		"lower":          strings.ToLower,
		"sub":            func(a, b int) int { return a - b },
		"urlquery":       url.QueryEscape,
	}
}

// FormatDiskSizeFloat formats a uint64 byte count as a human-readable string.
// Delegates to FormatFileSize with an overflow guard for disks > math.MaxInt64.
func FormatDiskSizeFloat(size uint64) string {
	if size > math.MaxInt64 {
		return FormatFileSize(math.MaxInt64)
	}
	return FormatFileSize(int64(size))
}

// FormatDiskPercent formats a float64 percentage to one decimal place.
func FormatDiskPercent(pct float64) string {
	return fmt.Sprintf(PercentFormat, pct)
}

// formatModTime formats a time.Time for display in file listings.
func formatModTime(t time.Time) string {
	return t.Format(ModTimeDisplayFormat)
}

// NewTemplateSet parses all templates from the given filesystem and returns
// a ready-to-use TemplateSet. Called once at server startup.
//
// Strategy: parse a base set (layout + all partials + all components), then
// for each page template, clone the base and parse the page into the clone.
//
// Uses path.Join (forward slashes) for embed FS paths, not filepath.Join,
// because embed.FS always uses forward slashes regardless of OS.
func NewTemplateSet(templateFS fs.FS) (*TemplateSet, error) {
	if templateFS == nil {
		return nil, fmt.Errorf("%s: nil filesystem", ErrMsgTemplateSet)
	}

	funcMap := templateFuncMap()

	// Collect partial and component template paths using path.Join (forward slashes for embed FS).
	sharedPaths := []string{
		path.Join(TemplateBaseDir, TemplatePartialsDir, TemplateBreadcrumbs),
		path.Join(TemplateBaseDir, TemplatePartialsDir, TemplateFilelist),
		path.Join(TemplateBaseDir, TemplatePartialsDir, TemplateDropzone),
		path.Join(TemplateBaseDir, TemplatePartialsDir, TemplateBookmarks),
		path.Join(TemplateBaseDir, TemplatePartialsDir, TemplateToast),
		path.Join(TemplateBaseDir, TemplatePartialsDir, TemplateDiskUsage),
		path.Join(TemplateBaseDir, TemplateComponentsDir, TemplatePreview),
	}

	layoutPath := path.Join(TemplateBaseDir, TemplateLayout)

	// Build base template: layout + all shared templates.
	allBasePaths := append([]string{layoutPath}, sharedPaths...)

	base, err := template.New(TemplateLayout).Funcs(funcMap).ParseFS(templateFS, allBasePaths...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMsgTemplateParse, err)
	}

	// For each page, clone the base and parse the page template into the clone.
	pageTemplates := map[string]string{
		PageLogin: path.Join(TemplateBaseDir, TemplateLogin),
		PageMain:  path.Join(TemplateBaseDir, TemplateMain),
	}

	pages := make(map[string]*template.Template, len(pageTemplates))
	for name, pagePath := range pageTemplates {
		clone, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("%s: clone for %s: %w", ErrMsgTemplateParse, name, err)
		}

		_, err = clone.ParseFS(templateFS, pagePath)
		if err != nil {
			return nil, fmt.Errorf("%s: page %s: %w", ErrMsgTemplateParse, name, err)
		}

		pages[name] = clone
	}

	return &TemplateSet{pages: pages}, nil
}

// RenderPage renders a full page (layout + content block) to the response writer.
// Renders into a buffer first to avoid committing headers on template errors.
func (ts *TemplateSet) RenderPage(w http.ResponseWriter, pageName string, statusCode int, data any) error {
	tmpl, ok := ts.pages[pageName]
	if !ok {
		return fmt.Errorf("%s: unknown page %q", ErrMsgTemplateRender, pageName)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, TemplateLayout, data); err != nil {
		return fmt.Errorf("%s: %w", ErrMsgTemplateRender, err)
	}

	w.Header().Set(HeaderContentType, ContentTypeHTML)
	w.WriteHeader(statusCode)
	_, _ = w.Write(buf.Bytes())

	return nil
}

// RenderPartial renders a named template block without the layout wrapper.
// Used for HTMX partial responses. Renders into a buffer first to avoid
// committing headers on template errors.
func (ts *TemplateSet) RenderPartial(w http.ResponseWriter, pageName string, blockName string, data any) error {
	tmpl, ok := ts.pages[pageName]
	if !ok {
		return fmt.Errorf("%s: unknown page %q", ErrMsgTemplateRender, pageName)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, blockName, data); err != nil {
		return fmt.Errorf("%s: %w", ErrMsgTemplateRender, err)
	}

	w.Header().Set(HeaderContentType, ContentTypeHTML)
	_, _ = w.Write(buf.Bytes())

	return nil
}

// BuildBreadcrumbs creates breadcrumb segments from a relative directory path.
// Always starts with "Home" pointing to the root.
// Example: "docs/2026/q1" → [Home(.), docs(docs), 2026(docs/2026), q1(docs/2026/q1)]
func BuildBreadcrumbs(relPath string) []BreadcrumbSegment {
	segments := []BreadcrumbSegment{
		{Label: BreadcrumbRootLabel, Path: DefaultBrowsePath},
	}

	cleaned := path.Clean(relPath)
	if cleaned == DefaultBrowsePath || cleaned == "" {
		return segments
	}

	parts := strings.Split(cleaned, "/")
	for i, part := range parts {
		if part == "" {
			continue
		}
		segPath := strings.Join(parts[:i+1], "/")
		segments = append(segments, BreadcrumbSegment{
			Label: part,
			Path:  segPath,
		})
	}

	return segments
}

// IsHTMXRequest checks whether the request was made by HTMX.
func IsHTMXRequest(r *http.Request) bool {
	return r.Header.Get(HeaderHXRequest) == HXRequestTrue
}
