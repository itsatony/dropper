package dropper

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// buildBrowseURL constructs a URL-safe browse redirect path.
func buildBrowseURL(relPath string) string {
	return RouteRoot + "?" + QueryParamPath + "=" + url.QueryEscape(relPath)
}

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

			// Render main page with error at root — show empty state with 403.
			data := PageData{
				CurrentPath: DefaultBrowsePath,
				Breadcrumbs: BuildBreadcrumbs(DefaultBrowsePath),
				SortBy:      sortBy,
				SortOrder:   sortOrder,
				Readonly:    cfg.Readonly,
				Error:       ErrMsgBrowsePath,
			}
			if renderErr := ts.RenderPage(w, PageMain, http.StatusForbidden, data); renderErr != nil {
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

			// HTMX request: return error as HTML partial (empty filelist with error breadcrumb).
			if IsHTMXRequest(r) {
				errData := PageData{
					CurrentPath: DefaultBrowsePath,
					Breadcrumbs: BuildBreadcrumbs(DefaultBrowsePath),
					SortBy:      sortBy,
					SortOrder:   sortOrder,
					Readonly:    cfg.Readonly,
					Error:       ErrMsgBrowsePath,
				}
				w.Header().Set(HeaderContentType, ContentTypeHTML)
				w.WriteHeader(http.StatusForbidden)
				if renderErr := ts.RenderPartial(w, PageMain, BlockBreadcrumbs, errData); renderErr != nil {
					logger.Error(ErrMsgTemplateRender, LogFieldError, renderErr)
					return
				}
				if renderErr := ts.RenderPartial(w, PageMain, BlockFilelist, errData); renderErr != nil {
					logger.Error(ErrMsgTemplateRender, LogFieldError, renderErr)
				}
				return
			}

			// Browser: redirect to root with error-safe URL.
			http.Redirect(w, r, buildBrowseURL(DefaultBrowsePath), http.StatusSeeOther)
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

		// Browser direct access — redirect to main page with URL-encoded path param.
		http.Redirect(w, r, buildBrowseURL(relPath), http.StatusSeeOther)
	}
}

// HandleDownload returns a handler for GET /files/download — serves a file for
// download with Content-Disposition: attachment. Validates path via SafePath,
// verifies the target is a file (not directory), and audit-logs the download.
func HandleDownload(cfg *DropperConfig, audit *AuditLogger, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		relPath := r.URL.Query().Get(QueryParamPath)
		if relPath == "" {
			RespondError(w, http.StatusBadRequest, ErrCodeBadRequest, ErrMsgMissingParam)
			return
		}

		safePath, err := SafePath(cfg.RootDir, relPath)
		if err != nil {
			logger.Warn(LogMsgPathDenied, LogFieldPath, relPath)
			RespondError(w, http.StatusForbidden, ErrCodeForbidden, ErrMsgPathTraversal)
			return
		}

		info, err := os.Stat(safePath)
		if err != nil {
			RespondError(w, http.StatusNotFound, ErrCodeNotFound, ErrMsgFileStat)
			return
		}
		if info.IsDir() {
			RespondError(w, http.StatusBadRequest, ErrCodeBadRequest, ErrMsgNotFile)
			return
		}

		// Audit log before serving — records intent even if download is interrupted.
		fileSize := info.Size()
		entry := NewAuditEntry(r, AuditActionDownload, relPath)
		entry.FileSize = &fileSize
		entry.Success = true
		audit.Log(entry)

		logger.Info(LogMsgDownloadServed,
			LogFieldPath, relPath,
			LogFieldFilename, info.Name(),
			LogFieldSize, fileSize,
		)

		// Set Content-Disposition before ServeFile writes headers.
		w.Header().Set(HeaderContentDisposition,
			fmt.Sprintf(ContentDispositionFormat, info.Name()))
		http.ServeFile(w, r, safePath)
	}
}

// HandleMkdir returns a handler for POST /files/mkdir — creates a new
// subdirectory. Validates readonly mode, sanitizes the name, and audit-logs.
func HandleMkdir(cfg *DropperConfig, audit *AuditLogger, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.Readonly {
			RespondError(w, http.StatusForbidden, ErrCodeReadonly, ErrMsgReadonlyMode)
			return
		}

		relPath := r.URL.Query().Get(QueryParamPath)
		if relPath == "" {
			relPath = DefaultBrowsePath
		}

		name := r.URL.Query().Get(QueryParamName)
		if name == "" {
			RespondError(w, http.StatusBadRequest, ErrCodeBadRequest, ErrMsgMissingParam)
			return
		}

		sanitizedName := SanitizeFilename(name)

		err := CreateDirectory(cfg.RootDir, relPath, name, cfg.Readonly, logger)
		if err != nil {
			entry := NewAuditEntry(r, AuditActionMkdir, filepath.Join(relPath, sanitizedName))
			entry.Success = false
			entry.Error = err.Error()
			audit.Log(entry)

			logger.Warn(LogMsgUploadFailed,
				LogFieldPath, relPath,
				LogFieldFilename, sanitizedName,
				LogFieldError, err,
			)

			statusCode := mapFSErrorToStatus(err)
			RespondError(w, statusCode, mapStatusToErrCode(statusCode), err.Error())
			return
		}

		entry := NewAuditEntry(r, AuditActionMkdir, filepath.Join(relPath, sanitizedName))
		entry.Success = true
		audit.Log(entry)

		logger.Info(LogMsgMkdirHandler,
			LogFieldPath, relPath,
			LogFieldFilename, sanitizedName,
		)

		RespondJSON(w, http.StatusCreated, MkdirResponse{
			Name: sanitizedName,
			Path: relPath,
		})
	}
}

// HandleUpload returns a handler for POST /files/upload — accepts multipart file
// uploads. Supports multiple files and clipboard mode (clipboard=true query param
// overrides filename with a timestamped clipboard name). Each file is written via
// SafeWriteFile which handles sanitization, extension validation, and collision
// resolution. All uploads are audit-logged.
func HandleUpload(cfg *DropperConfig, audit *AuditLogger, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.Readonly {
			RespondError(w, http.StatusForbidden, ErrCodeReadonly, ErrMsgReadonlyMode)
			return
		}

		// Wrap body with MaxBytesReader before parsing multipart form.
		r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxUploadBytes)

		if err := r.ParseMultipartForm(MaxMultipartMemory); err != nil {
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				RespondError(w, http.StatusRequestEntityTooLarge, ErrCodePayloadTooLarge, ErrMsgBodyTooLarge)
				return
			}
			logger.Warn(LogMsgUploadFailed, LogFieldError, err)
			RespondError(w, http.StatusBadRequest, ErrCodeBadRequest, ErrMsgMultipartParse)
			return
		}
		defer func() {
			if r.MultipartForm != nil {
				_ = r.MultipartForm.RemoveAll()
			}
		}()

		relPath := r.URL.Query().Get(QueryParamPath)
		if relPath == "" {
			relPath = DefaultBrowsePath
		}

		isClipboard := r.URL.Query().Get(QueryParamClipboard) == QueryParamClipboardTrue

		files := r.MultipartForm.File[FormFieldFile]
		if len(files) == 0 {
			RespondError(w, http.StatusBadRequest, ErrCodeBadRequest, ErrMsgNoFilesUploaded)
			return
		}

		results := make([]UploadResult, 0, len(files))
		uploaded := 0
		failed := 0

		for i, fh := range files {
			file, err := fh.Open()
			if err != nil {
				results = append(results, UploadResult{
					OriginalName: fh.Filename,
					Error:        err.Error(),
				})
				failed++
				continue
			}

			// Determine filename: clipboard mode overrides first file's name.
			filename := fh.Filename
			if isClipboard && i == 0 {
				filename = ClipboardFilename()
			}

			finalName, err := SafeWriteFile(
				cfg.RootDir, relPath, filename, file,
				cfg.MaxUploadBytes, cfg.AllowedExtensions,
				cfg.Readonly, logger,
			)
			file.Close()

			if err != nil {
				entry := NewAuditEntry(r, AuditActionUpload, filepath.Join(relPath, filename))
				entry.Success = false
				entry.Error = err.Error()
				audit.Log(entry)

				results = append(results, UploadResult{
					OriginalName: fh.Filename,
					Error:        err.Error(),
				})
				failed++

				logger.Warn(LogMsgUploadFailed,
					LogFieldFilename, filename,
					LogFieldPath, relPath,
					LogFieldError, err,
				)
				continue
			}

			// Stat the written file to get actual size.
			var fileSize int64
			safePath, statErr := SafePath(cfg.RootDir, filepath.Join(relPath, finalName))
			if statErr == nil {
				if info, statErr := os.Stat(safePath); statErr == nil {
					fileSize = info.Size()
				}
			}

			entry := NewAuditEntry(r, AuditActionUpload, filepath.Join(relPath, finalName))
			entry.FileSize = &fileSize
			entry.Success = true
			audit.Log(entry)

			if isClipboard && i == 0 {
				logger.Info(LogMsgPasteUpload,
					LogFieldFilename, finalName,
					LogFieldPath, relPath,
					LogFieldSize, fileSize,
				)
			} else {
				logger.Info(LogMsgUploadSuccess,
					LogFieldFilename, finalName,
					LogFieldPath, relPath,
					LogFieldSize, fileSize,
				)
			}

			results = append(results, UploadResult{
				OriginalName: fh.Filename,
				FinalName:    finalName,
				Size:         fileSize,
			})
			uploaded++
		}

		logger.Info(LogMsgUploadSuccess,
			LogFieldUploadCount, uploaded,
			LogFieldFailCount, failed,
		)

		RespondOK(w, UploadResponse{
			Results:  results,
			Uploaded: uploaded,
			Failed:   failed,
		})
	}
}

// mapFSErrorToStatus maps filesystem operation errors to HTTP status codes.
func mapFSErrorToStatus(err error) int {
	msg := err.Error()
	switch {
	case strings.Contains(msg, ErrMsgPathTraversal):
		return http.StatusForbidden
	case strings.Contains(msg, ErrMsgReadonlyMode):
		return http.StatusForbidden
	case strings.Contains(msg, ErrMsgExtNotAllowed):
		return http.StatusBadRequest
	case strings.Contains(msg, ErrMsgNotDirectory):
		return http.StatusBadRequest
	case strings.Contains(msg, ErrMsgFileTooLarge):
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusInternalServerError
	}
}

// mapStatusToErrCode maps HTTP status codes to error code strings.
func mapStatusToErrCode(status int) string {
	switch status {
	case http.StatusForbidden:
		return ErrCodeForbidden
	case http.StatusBadRequest:
		return ErrCodeBadRequest
	case http.StatusRequestEntityTooLarge:
		return ErrCodePayloadTooLarge
	default:
		return ErrCodeInternal
	}
}
