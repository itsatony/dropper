# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.10.0] - 2026-04-06

### Added
- Directory upload with folder structure preservation via `webkitGetAsEntry` API
- `SafeWriteFileWithRelPath` for secure nested directory creation during uploads
- `relpath` multipart form field for directory upload relative path preservation
- Disk usage footer in main view (async HTMX load from `/healthz`)
- HTMX content negotiation on `/healthz` endpoint (HTML partial for HTMX, JSON for API)
- Last-directory auto-navigation on page load (localStorage-based)
- Upload progress bar with real-time percentage (XHR `upload.onprogress`)
- `.golangci.yml` linting configuration (errcheck, govet, staticcheck, gosec, gocritic, gofmt, misspell)
- `diskusage.html` template partial with disk bar visualization
- Progress bar container in dropzone template
- `ErrInvalidRelPath` sentinel error and constructor
- DC-10 unit tests: `SafeWriteFileWithRelPath` (10 cases), directory upload handlers (4 cases)
- DC-10 integration tests: directory upload workflow, disk usage HTMX rendering
- DC-10 health tests: HTMX response, JSON preservation, invalid path handling

### Changed
- `HandleHealthz` now accepts `*TemplateSet` parameter for HTMX partial rendering
- `HandleUpload` reads parallel `relpath` form fields for directory structure uploads
- `dropper.js` rewritten upload function from `fetch()` to `XMLHttpRequest` for progress events
- `dropper.js` drop handler uses `webkitGetAsEntry` with recursive directory traversal
- Dropzone text updated to mention folder support
- `PageData` struct includes optional `DiskUsage` field
- Test template FS updated with `diskusage.html` partial

## [0.9.0] - 2026-04-06

### Added
- Request logging middleware with configurable `no_log_paths` exclusion
- `DROPPER_LOGGING_NO_LOG_PATHS` env var for request log path filtering
- Path length guard in SafePath (reject paths > 4096 chars)
- Config validation: `root_dir` must exist, `allowed_extensions` must start with `.`, `audit_log_path` parent must exist
- ADR-005: Symlink TOCTOU limitation documented
- Integration tests: full mkdir+upload+list+download workflow, concurrent uploads, clipboard upload, extension rejection, null byte injection, path traversal, security headers on all route types, request logging NoLogPaths
- Filesystem tests: path length guard, concurrent writes, sequential collision resolution, non-existent nested paths
- Audit tests: heavy concurrent writes (10 goroutines x 100 entries)
- Config tests: extension format validation, root dir existence, audit log path parent, NoLogPaths env var binding

### Changed
- Test coverage increased from 87.5% to 88.2% (218 tests, up from 176)
- Config loading now validates filesystem state (root_dir, audit_log_path parent)
- `ErrPathTooLong` sentinel error and `NewPathTooLongError()` constructor added

## [0.8.0] - 2026-04-06

### Added
- Dockerfile with multi-stage build (golang:1.25-alpine build, alpine:3.21 runtime)
- docker-compose.yaml with healthcheck, volume mounts, env var substitution
- Interactive setup script (`scripts/setup.sh`) with runtime detection, config generation
- Container E2E smoke test (`scripts/smoke_test.sh`)
- README.md with full project documentation
- CHANGELOG.md (this file)
- ADRs.md with architecture decision records
- Makefile targets: docker-stop, docker-logs, smoke-test
- .dockerignore for minimal image size

### Changed
- Makefile docker-build target now passes version build args
- Makefile docker-run target uses named container
- Makefile setup target now runs scripts/setup.sh

## [0.7.0] - 2026-04-05

### Added
- Custom `DropperError` type with sentinel errors, HTTP status codes, and safe messages
- Prometheus custom metrics: `dropper_http_requests_total`, `dropper_uploads_total`, `dropper_upload_bytes_total`, `dropper_errors_total`
- `MetricsMiddleware` for automatic request counting with method/route/status labels
- Error count instrumentation in `RespondError`
- Upload metrics instrumentation in `HandleUpload`
- HTMX content negotiation tests, sorting tests, breadcrumb tests

### Removed
- Obsolete `fsErrorMapping` / `mapFSError` / `safeUploadErrorMessage` (replaced by DropperError)

## [0.6.0] - 2026-04-04

### Added
- File upload handler (`POST /files/upload`) with multipart support, MaxBytesReader
- File download handler (`GET /files/download`) with Content-Disposition
- Directory creation handler (`POST /files/mkdir`) with sanitization
- Clipboard paste mode via `?clipboard=true` parameter
- Read-only mode enforcement on all write operations
- Per-file `io.LimitReader` for defense-in-depth upload size limiting
- `dropper.js` upload XHR, drop handler, file input, clipboard confirm, mkdir button
- Integration tests for full upload-list-download flow
- Audit logging in all file operation handlers

## [0.5.0] - 2026-04-03

### Added
- `TemplateSet` with pre-compiled page clones and FuncMap helpers
- `templates/layout.html` base layout with scripts, toast container
- `templates/login.html` login form
- `templates/main.html` main file browser view
- Partials: breadcrumbs, filelist, dropzone, bookmarks, toast
- Components: image preview modal
- Static assets: vendored HTMX, dropper.css (light/dark mode), dropper.js
- Security headers middleware (CSP, X-Frame-Options, X-Content-Type-Options)
- HTMX partial page updates for directory navigation

## [0.4.0] - 2026-04-02

### Added
- Append-only JSON lines audit logger (`dropper.audit.go`)
- Configurable audit log path (empty = disabled)
- Audit entries: timestamp, client IP, action, path, file size, success/failure
- Log rotation support via `Reopen()` method
- Thread-safe concurrent write handling

## [0.3.0] - 2026-04-01

### Added
- In-memory session store with TTL and background cleanup
- Per-IP sliding window rate limiter (configurable, default 5/min)
- Session token generation via `crypto/rand` (32-byte hex)
- Login page handler, login POST handler, logout handler
- Session middleware with content negotiation (JSON 401 / HTML 303)
- `crypto/subtle.ConstantTimeCompare` for timing-safe secret validation
- Rate limiting on login endpoint

## [0.2.0] - 2026-03-31

### Added
- Path jail algorithm: Clean, Abs, HasPrefix, EvalSymlinks, re-check
- Filename sanitization to `[a-zA-Z0-9_.-]`
- Collision resolution with `{YYYYMMDD}-{HHmmss}_` prefix
- Safe file write: temp dir write then rename into place
- Extension whitelist validation
- Directory listing with file metadata (name, size, mod time, is-dir)
- Human-readable file size formatting

## [0.1.0] - 2026-03-30

### Added
- Project skeleton with Go module setup
- Config system (viper + validator, YAML + env overrides)
- Structured logging via slog (JSON/console, stdout/stderr)
- Chi router setup with middleware chain (Recoverer, RequestID, RealIP)
- Health endpoint (`/healthz`) with disk usage reporting
- Version endpoint (`/version`) via go-version
- Prometheus metrics endpoint (`/metrics`)
- JSON response helpers
- Constants file (zero magic literals)
- Embedded assets via go:embed (versions.yaml, static/*, templates/*)
- Makefile with build, run, test, lint targets
