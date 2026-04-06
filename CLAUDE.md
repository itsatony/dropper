# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Dropper is a minimal, self-hosted file drop-zone web tool. Upload files via drag-and-drop or clipboard paste, browse and download from a local directory tree. Deployed as a container with mounted volumes — no SFTP, no SSH, no remote connections. The "remote" is a deployment concern (bind mount, NFS, PVC).

**Status:** Cycle 10 complete (spec completion: directory upload, disk usage footer, last-dir navigation, upload progress, linting config). Building from spec (`project_plan/dropper_spec.md`).
**Stack:** Go 1.25+ / chi/v5 / HTMX / server-rendered HTML templates
**License:** Apache 2.0

## Build & Dev Commands

```bash
make build            # go build
make run              # go run (dev mode, console logging)
make test             # go test -race -cover ./...
make lint             # golangci-lint run
make docker-build     # build container image
make docker-run       # run container with example config
make docker-stop      # stop dev container
make docker-logs      # tail container logs
make setup            # interactive setup script
make smoke-test       # container E2E smoke test
```

Run a single test:
```bash
go test -race -run TestSafePath ./internal/...
```

## Architecture

Single Go binary serves everything (API + static assets + templates). No frontend build step.

```
Browser (HTMX) → HTTPS (reverse proxy) → dropper (Go/chi) → /data (mounted volume)
```

### Code Layout

- `cmd/dropper/main.go` — entrypoint, signal handling
- `internal/` — flat package, files named `dropper.{concern}.go`:
  - `dropper.constants.go` — all string/numeric constants (zero magic literals)
  - `dropper.config.go` — config struct + validation (viper + validator)
  - `dropper.logging.go` — slog logger factory (JSON/console, stdout/stderr)
  - `dropper.response.go` — JSON response helpers
  - `dropper.server.go` — chi router setup, middleware wiring, server lifecycle
  - `dropper.handlers.auth.go` — login, logout, session middleware, content negotiation
  - `dropper.handlers.files.go` — file browser (GET /, GET /files), upload (POST /files/upload), download (GET /files/download), mkdir (POST /files/mkdir)
  - `dropper.handlers.health.go` — /healthz (disk usage), /metrics (prometheus)
  - `dropper.errors.go` — DropperError type, sentinel errors, MapDropperError for HTTP error mapping
  - `dropper.metrics.go` — Prometheus custom metrics (request count, upload count/bytes, error count), MetricsMiddleware
  - `dropper.fs.go` — filesystem operations, path jail
  - `dropper.session.go` — in-memory session store with TTL, crypto/rand tokens, cleanup goroutine
  - `dropper.ratelimit.go` — per-IP sliding window rate limiter
  - `dropper.audit.go` — append-only JSON lines audit logger, log rotation support
  - `dropper.templates.go` — TemplateSet with pre-compiled page sets, render helpers, breadcrumbs, HTMX detection
- `embed.go` — root-level `//go:embed` for versions.yaml, static/*, templates/*
- `templates/` — Go HTML templates (layout, login, main, partials/, components/)
- `static/` — vendored HTMX, dropper.js (clipboard/drag-drop/localStorage), dropper.css
- `versions.yaml` — go-version source of truth

### Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/go-chi/chi/v5` | Router + middleware |
| `github.com/spf13/viper` | Config loading (YAML + env overrides) |
| `github.com/go-playground/validator/v10` | Config struct validation |
| `github.com/itsatony/go-version` | Embedded versions.yaml, /version handler |
| `log/slog` (stdlib) | Structured logging (JSON prod, text dev) |
| `github.com/prometheus/client_golang` | Prometheus metrics |
| `github.com/stretchr/testify` | Test assertions |

## Critical Security Constraints

**Path traversal prevention is the #1 security surface.** Every client-supplied path must be:
1. `filepath.Clean` → `filepath.Abs` → `strings.HasPrefix(root)` check
2. `filepath.EvalSymlinks` → re-check against root
3. Rejection returns 403 with zero path info in response

Upload safety: temp dir write → rename into place; filename sanitized to `[a-zA-Z0-9_.-]`; extension whitelist checked before any disk write; `http.MaxBytesReader` enforced.

Auth: `crypto/subtle.ConstantTimeCompare` for secret; `crypto/rand` for session tokens; rate-limited login (5/min/IP).

## Design Decisions

- **No delete** — intentional one-way drop zone with read access
- **In-memory sessions** — lost on restart, acceptable for scope
- **All deps must be public** — open-source project, no private vAI libraries
- **HTMX over SPA** — no node_modules, no frontend build, single binary
- **No TLS termination** — reverse proxy handles it
- **Filename collisions** — prefix with `{YYYYMMDD}-{HHmmss}_`, never overwrite, never reject
- **Read-only mode** — config flag hides upload UI, disables write endpoints

## Config

Via viper (YAML + env overrides). Env vars prefixed `DROPPER_` (e.g., `DROPPER_SECRET`, `DROPPER_ROOT_DIR`). See `configs/dropper.example.yaml` and spec section 5 for full schema.

## Testing Strategy

- Table-driven tests, always `go test -race`
- Heavy unit test coverage on: path jail logic, config validation, filename sanitization, collision resolution, session TTL, audit log formatting
- Integration tests for: full HTTP cycle (login → upload → list → download), readonly mode, extension filtering, rate limiting
- Target: >80% coverage on `internal/`
