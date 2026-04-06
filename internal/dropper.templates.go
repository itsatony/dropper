package dropper

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
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
		"formatSize": FormatFileSize,
		"formatTime": formatModTime,
		"lower":      strings.ToLower,
		"sub":        func(a, b int) int { return a - b },
	}
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
func NewTemplateSet(templateFS fs.FS) (*TemplateSet, error) {
	if templateFS == nil {
		return nil, fmt.Errorf("%s: nil filesystem", ErrMsgTemplateSet)
	}

	funcMap := templateFuncMap()

	// Collect partial and component template paths.
	sharedPaths := []string{
		filepath.Join(TemplateBaseDir, TemplatePartialsDir, TemplateBreadcrumbs),
		filepath.Join(TemplateBaseDir, TemplatePartialsDir, TemplateFilelist),
		filepath.Join(TemplateBaseDir, TemplatePartialsDir, TemplateDropzone),
		filepath.Join(TemplateBaseDir, TemplatePartialsDir, TemplateBookmarks),
		filepath.Join(TemplateBaseDir, TemplatePartialsDir, TemplateToast),
		filepath.Join(TemplateBaseDir, TemplateComponentsDir, TemplatePreview),
	}

	// Parse the layout as the base template with function map.
	layoutPath := filepath.Join(TemplateBaseDir, TemplateLayout)

	// Build base template: layout + all shared templates.
	allBasePaths := append([]string{layoutPath}, sharedPaths...)

	base, err := template.New(TemplateLayout).Funcs(funcMap).ParseFS(templateFS, allBasePaths...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMsgTemplateParse, err)
	}

	// For each page, clone the base and parse the page template into the clone.
	pageTemplates := map[string]string{
		PageLogin: filepath.Join(TemplateBaseDir, TemplateLogin),
		PageMain:  filepath.Join(TemplateBaseDir, TemplateMain),
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
// Sets Content-Type to HTML and writes the given status code.
func (ts *TemplateSet) RenderPage(w http.ResponseWriter, pageName string, statusCode int, data any) error {
	tmpl, ok := ts.pages[pageName]
	if !ok {
		return fmt.Errorf("%s: unknown page %q", ErrMsgTemplateRender, pageName)
	}

	w.Header().Set(HeaderContentType, ContentTypeHTML)
	w.WriteHeader(statusCode)

	if err := tmpl.ExecuteTemplate(w, TemplateLayout, data); err != nil {
		return fmt.Errorf("%s: %w", ErrMsgTemplateRender, err)
	}

	return nil
}

// RenderPartial renders a named template block without the layout wrapper.
// Used for HTMX partial responses.
func (ts *TemplateSet) RenderPartial(w http.ResponseWriter, pageName string, blockName string, data any) error {
	tmpl, ok := ts.pages[pageName]
	if !ok {
		return fmt.Errorf("%s: unknown page %q", ErrMsgTemplateRender, pageName)
	}

	w.Header().Set(HeaderContentType, ContentTypeHTML)

	if err := tmpl.ExecuteTemplate(w, blockName, data); err != nil {
		return fmt.Errorf("%s: %w", ErrMsgTemplateRender, err)
	}

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
