# Dropper — Masterplan

Build order derived from the spec (`dropper_spec.md`) and CLAUDE.md cycle annotations.
Each cycle is a coherent, testable increment.

## Cycle 1 — Project Skeleton (DONE)
- Config struct + validation (viper + validator)
- Structured logging (slog, JSON/console)
- chi router setup, middleware wiring, server lifecycle
- Health handler (`/healthz` with disk usage)
- Version handler (`/version` via go-version)
- Prometheus metrics (`/metrics`)
- Response helpers (JSON)
- Constants file
- Embed setup (versions.yaml, static/*, templates/*)
- Makefile, Dockerfile scaffolding
- **Coverage:** 89.7%

## Cycle 2 — Filesystem & Path Jail (DONE)
- `dropper.fs.go` — core filesystem operations
- Path jail: Clean → Abs → HasPrefix → EvalSymlinks → re-check
- Filename sanitization (`[a-zA-Z0-9_.-]`, replace rest with `_`)
- Collision resolution: `{YYYYMMDD}-{HHmmss}_` prefix on conflict
- Safe file write: temp dir → rename into place
- Extension whitelist check
- Directory listing (name, size, mod time, is-dir)
- `dropper.fs_test.go` — heavy unit tests (security-critical surface)

## Cycle 3 — Authentication & Sessions (DONE)
- `dropper.session.go` — in-memory session store with TTL, mutex-guarded, background cleanup
- `dropper.ratelimit.go` — per-IP sliding window rate limiter (inline pruning)
- Session token generation (crypto/rand, 32-byte hex)
- `dropper.handlers.auth.go` — login page, login POST, logout, session middleware, content negotiation
- Session middleware (cookie check → session lookup → 401 JSON / 303 redirect)
- Rate limiting on login (5/min/IP, configurable)
- `crypto/subtle.ConstantTimeCompare` for secret
- `templates/login.html` — login form extending layout
- 50 tests, 85.9% coverage, race-detector clean
- **Version:** 0.3.0

## Cycle 4 — Audit Logging
- `dropper.audit.go` — append-only JSON lines logger
- Records: timestamp, client IP, action, path, file size, success/failure
- Configurable log path
- No audit of browse/list (spec: too noisy)
- Tests for log formatting, rotation-friendliness

## Cycle 5 — Templates & Static Assets
- `dropper.templates.go` — template loading, rendering helpers
- `templates/layout.html` — base layout (head, scripts, toast container)
- `templates/login.html` — login form
- `templates/main.html` — main file browser view
- `templates/partials/` — breadcrumbs, filelist, dropzone, bookmarks, toast
- `templates/components/preview.html` — image preview modal
- Static assets: vendored HTMX, dropper.css, dropper.js (stubs)
- Security headers middleware (CSP, X-Frame-Options, nosniff)

## Cycle 6 — File Handlers (Upload, Download, Mkdir) (DONE)
- `dropper.handlers.files.go` — HandleUpload, HandleDownload, HandleMkdir
- `POST /files/upload?path=` — multipart upload with MaxBytesReader, clipboard mode via `?clipboard=true`
- `GET /files/download?path=` — file download with Content-Disposition, audit logging
- `POST /files/mkdir?path=&name=` — directory creation with sanitization, audit logging
- Paste merged into upload (clipboard blob sent as FormData, no separate endpoint)
- All handlers return JSON only; JS client uses `htmx.ajax()` to refresh file list
- `http.MaxBytesReader` + per-file `io.LimitReader` (defense in depth)
- Readonly mode enforcement on all write operations
- `dropper.js` — upload XHR, drop handler, file input, clipboard confirm, mkdir button, refreshFileList
- Templates — mkdir button in main.html, data-upload-url on dropzone
- 29 new tests (176 total), 86.3% coverage, race-detector clean
- Integration tests: full upload→list→download flow, readonly mode, auth required, audit log verification
- **Version:** 0.6.0

## Cycle 7 — Custom Errors, Prometheus Metrics, HTMX Test Hardening (DONE)
- HTMX UI features (drag-drop, clipboard paste, breadcrumbs, sorting, bookmarks, toast, preview, dark mode) — completed in cycles 5-6
- `dropper.errors.go` — DropperError type with sentinel errors, replaces string-matching `mapFSError()`
- `dropper.metrics.go` — Prometheus custom metrics: `dropper_http_requests_total`, `dropper_uploads_total`, `dropper_upload_bytes_total`, `dropper_errors_total`
- MetricsMiddleware — chi middleware for request counting with method/route/status labels
- Error count instrumentation in RespondError
- Upload metrics instrumentation in HandleUpload
- Removed obsolete `fsErrorMapping` / `mapFSError` / `safeUploadErrorMessage`
- New test suites: dropper.errors_test.go, dropper.metrics_test.go
- Additional HTMX content negotiation, sorting, breadcrumb, and E2E tests
- **Version:** 0.7.0

## Cycle 8 — Docker, Setup Script, Polish (DONE)
- Dockerfile (multi-stage build: golang:1.25-alpine → alpine:3.21, build args for version injection)
- .dockerignore for minimal image size
- docker-compose.yaml (healthcheck, volume mounts, env var substitution)
- `scripts/setup.sh` — interactive setup (runtime detection, port check, prompts, config generation, build, start)
- `scripts/smoke_test.sh` — container E2E smoke test (9 tests: health, version, auth flow, metrics)
- Makefile targets: docker-build (with build args), docker-run, docker-stop, docker-logs, setup, smoke-test
- README.md (features, quick start, config reference, deployment, architecture, API, security)
- CHANGELOG.md (all cycles 0.1.0–0.8.0)
- ADRs.md (4 architecture decision records)
- **Version:** 0.8.0

## Cycle 9 — Final Integration & Release Prep (DONE)
- Request logging middleware wired (`no_log_paths` config feature, env var binding)
- Path length guard added to SafePath (4096 char limit)
- Config validation: root_dir existence, allowed_extensions format, audit_log_path parent
- Full integration test suite: 15+ new tests (workflows, concurrency, security edge cases)
- Coverage: 88.2% aggregate on internal/ (218 tests total)
- Security review: null byte injection, path traversal, TOCTOU documentation (ADR-005)
- Config audit: env var binding for NoLogPaths, all config keys verified
- ADR-005: Symlink TOCTOU limitation documented
- Documentation updated: README config table, CHANGELOG, CLAUDE.md
- **Version:** 0.9.0

## Cycle 10 — Spec Completion & Final Polish (DONE)
- Directory upload with folder structure preservation (webkitGetAsEntry API, recursive traversal)
- `SafeWriteFileWithRelPath`: per-component sanitization, intermediate dir creation, root jail enforcement
- `HandleUpload` reads parallel `relpath` form fields for nested directory uploads
- Disk usage footer in main view via async HTMX load from `/healthz`
- HTMX content negotiation on `/healthz` (HTML partial vs JSON)
- Last-directory auto-navigation via localStorage on page load
- Upload progress bar (XHR `upload.onprogress`, CSS progress fill)
- `.golangci.yml` linting config (errcheck, govet, staticcheck, gosec, gocritic, gofmt, misspell)
- DC-10 tests: 20+ new tests (unit, handler, integration)
- Coverage: 88.1% aggregate on internal/
- **Version:** 0.10.0
