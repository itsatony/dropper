package dropper

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// HandleMainPage returns a handler for GET / — the authenticated file browser.
// Renders the main page template with the directory listing for the requested path.
func HandleMainPage(ts *TemplateSet, cfg *DropperConfig, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relPath := r.URL.Query().Get(QueryParamPath)
		if relPath == "" {
			relPath = DefaultBrowsePath
		}

		sortBy := r.URL.Query().Get(QueryParamSortBy)
		if sortBy == "" {
			sortBy = DefaultSortField
		}
		sortOrder := r.URL.Query().Get(QueryParamSortOrder)
		if sortOrder == "" {
			sortOrder = DefaultSortOrder
		}

		entries, err := ListDirectory(cfg.RootDir, relPath, sortBy, sortOrder)
		if err != nil {
			logger.Warn(LogMsgBrowseDenied,
				LogFieldBrowsePath, relPath,
				LogFieldError, err,
			)

			if wantsJSON(r) {
				RespondError(w, http.StatusForbidden, ErrCodeForbidden, ErrMsgBrowsePath)
				return
			}

			// Render main page with error — show empty state.
			data := PageData{
				CurrentPath: DefaultBrowsePath,
				Breadcrumbs: BuildBreadcrumbs(DefaultBrowsePath),
				SortBy:      sortBy,
				SortOrder:   sortOrder,
				Readonly:    cfg.Readonly,
				Error:       ErrMsgBrowsePath,
			}
			if renderErr := ts.RenderPage(w, PageMain, http.StatusOK, data); renderErr != nil {
				logger.Error(ErrMsgTemplateRender, LogFieldError, renderErr)
			}
			return
		}

		data := PageData{
			CurrentPath: relPath,
			Entries:     entries,
			Breadcrumbs: BuildBreadcrumbs(relPath),
			SortBy:      sortBy,
			SortOrder:   sortOrder,
			Readonly:    cfg.Readonly,
		}

		if renderErr := ts.RenderPage(w, PageMain, http.StatusOK, data); renderErr != nil {
			logger.Error(ErrMsgTemplateRender, LogFieldError, renderErr)
		}
	}
}

// HandleListFiles returns a handler for GET /files — directory listing with
// three-way content negotiation:
//   - HTMX request (HX-Request header): render filelist + breadcrumbs partials
//   - JSON request (Accept: application/json): return JSON file listing
//   - Browser request: redirect to /?path=...
func HandleListFiles(ts *TemplateSet, cfg *DropperConfig, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relPath := r.URL.Query().Get(QueryParamPath)
		if relPath == "" {
			relPath = DefaultBrowsePath
		}

		sortBy := r.URL.Query().Get(QueryParamSortBy)
		if sortBy == "" {
			sortBy = DefaultSortField
		}
		sortOrder := r.URL.Query().Get(QueryParamSortOrder)
		if sortOrder == "" {
			sortOrder = DefaultSortOrder
		}

		entries, err := ListDirectory(cfg.RootDir, relPath, sortBy, sortOrder)
		if err != nil {
			logger.Warn(LogMsgBrowseDenied,
				LogFieldBrowsePath, relPath,
				LogFieldError, err,
			)

			if wantsJSON(r) {
				RespondError(w, http.StatusForbidden, ErrCodeForbidden, ErrMsgBrowsePath)
				return
			}

			RespondError(w, http.StatusForbidden, ErrCodeForbidden, ErrMsgBrowsePath)
			return
		}

		// JSON response.
		if wantsJSON(r) {
			w.Header().Set(HeaderContentType, ContentTypeJSON)
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(entries); err != nil {
				logger.Error(ErrMsgJSONEncode, LogFieldError, err)
			}
			return
		}

		// HTMX partial response — render breadcrumbs + filelist without layout.
		if IsHTMXRequest(r) {
			data := PageData{
				CurrentPath: relPath,
				Entries:     entries,
				Breadcrumbs: BuildBreadcrumbs(relPath),
				SortBy:      sortBy,
				SortOrder:   sortOrder,
				Readonly:    cfg.Readonly,
			}

			w.Header().Set(HeaderContentType, ContentTypeHTML)
			if renderErr := ts.RenderPartial(w, PageMain, BlockBreadcrumbs, data); renderErr != nil {
				logger.Error(ErrMsgTemplateRender, LogFieldError, renderErr)
				return
			}
			if renderErr := ts.RenderPartial(w, PageMain, BlockFilelist, data); renderErr != nil {
				logger.Error(ErrMsgTemplateRender, LogFieldError, renderErr)
			}
			return
		}

		// Browser direct access — redirect to main page with path param.
		http.Redirect(w, r, RouteRoot+"?"+QueryParamPath+"="+relPath, http.StatusSeeOther)
	}
}
