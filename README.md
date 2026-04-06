# dropper

A minimal, self-hosted file drop-zone web tool.

Upload files via drag-and-drop or clipboard paste, browse and download from a local directory tree. Deployed as a container with mounted volumes — no SFTP, no SSH, no remote connections. The "remote" is a deployment concern (bind mount, NFS, PVC).

**Stack:** Go 1.25+ / chi/v5 / HTMX / server-rendered HTML templates
**Module:** `github.com/vAudience/dropper`
**License:** Apache 2.0

---

## Features

- Drag-and-drop file upload (files and directories)
- Clipboard paste for screenshots (Ctrl+V)
- Directory browsing with breadcrumb navigation
- File download (click to download)
- Directory creation
- Bookmarks (localStorage)
- Sorting by name, date, size
- Read-only mode (config flag)
- Pre-shared secret authentication with session cookies
- Rate-limited login (5 attempts/min/IP)
- Audit logging (JSON lines, append-only)
- Prometheus metrics (/metrics)
- Health endpoint (/healthz with disk usage)
- Light/dark mode (system preference)
- Single binary, no frontend build step

---

## Quick Start

### Docker (recommended)

```bash
git clone https://github.com/vAudience/dropper.git
cd dropper
bash scripts/setup.sh
```

The setup script detects your container runtime, prompts for configuration, builds the image, and starts the container.

### Manual

```bash
git clone https://github.com/vAudience/dropper.git
cd dropper
cp configs/dropper.example.yaml configs/dropper.local.yaml
# Edit configs/dropper.local.yaml — set your secret
make run
```

---

## Configuration

Config is loaded from YAML file + env var overrides. Env vars take precedence.

| Key | Env Var | Default | Description |
|-----|---------|---------|-------------|
| `dropper.listen_port` | `DROPPER_LISTEN_PORT` | `8080` | HTTP listen port |
| `dropper.secret` | `DROPPER_SECRET` | — (required) | Pre-shared authentication secret (min 8 chars) |
| `dropper.session_ttl` | `DROPPER_SESSION_TTL` | `24h` | Session duration |
| `dropper.rate_limit_login` | `DROPPER_RATE_LIMIT_LOGIN` | `5` | Max login attempts per minute per IP |
| `dropper.root_dir` | `DROPPER_ROOT_DIR` | `./data` | Root directory to serve |
| `dropper.readonly` | `DROPPER_READONLY` | `false` | Disable uploads and mkdir |
| `dropper.max_upload_bytes` | `DROPPER_MAX_UPLOAD_BYTES` | `104857600` | Max upload size (100MB) |
| `dropper.allowed_extensions` | `DROPPER_ALLOWED_EXTENSIONS` | `[]` (all) | Whitelist of allowed file extensions |
| `dropper.audit_log_path` | `DROPPER_AUDIT_LOG_PATH` | `dropper_audit.log` | Audit log file path |
| `dropper.logging.level` | `DROPPER_LOGGING_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `dropper.logging.format` | `DROPPER_LOGGING_FORMAT` | `json` | Log format (json, console) |
| `dropper.logging.output` | — | `stdout` | Log output (stdout, stderr) |

---

## Deployment

### Docker Compose

```yaml
services:
  dropper:
    image: vaudience/dropper:latest
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./configs/dropper.yaml:/etc/dropper/dropper.yaml:ro
      - ./data:/data
      - dropper-audit:/var/log
    environment:
      - DROPPER_SECRET=your-secret-here
    command: ["--config", "/etc/dropper/dropper.yaml"]
volumes:
  dropper-audit:
```

### Reverse Proxy (nginx)

```nginx
server {
    listen 443 ssl;
    server_name drop.example.com;
    ssl_certificate     /etc/ssl/certs/drop.crt;
    ssl_certificate_key /etc/ssl/private/drop.key;
    client_max_body_size 100m;
    location / {
        proxy_pass http://dropper:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Reverse Proxy (Caddy)

```
drop.example.com {
    reverse_proxy dropper:8080
}
```

---

## Development

```bash
make build            # Build binary
make run              # Run in dev mode (console logging)
make test             # Run tests with race detector
make lint             # Run golangci-lint
make docker-build     # Build container image
make docker-run       # Run container with example config
make smoke-test       # Run container E2E smoke test
make setup            # Interactive setup script
```

---

## Architecture

```
Browser (HTMX) --> HTTPS (reverse proxy) --> dropper (Go/chi) --> /data (mounted volume)
```

Single Go binary serves everything (API + static assets + templates). No frontend build step.

### Code Layout

```
cmd/dropper/main.go         -- entrypoint, signal handling
internal/                    -- flat package, files named dropper.{concern}.go
  dropper.config.go          -- config struct + validation
  dropper.server.go          -- chi router setup, middleware
  dropper.handlers.auth.go   -- login, logout, session middleware
  dropper.handlers.files.go  -- upload, download, mkdir, browse
  dropper.handlers.health.go -- /healthz, /metrics
  dropper.fs.go              -- filesystem operations, path jail
  dropper.session.go         -- in-memory session store
  dropper.ratelimit.go       -- per-IP rate limiter
  dropper.audit.go           -- JSON lines audit logger
  dropper.errors.go          -- custom error types
  dropper.metrics.go         -- Prometheus metrics
  dropper.templates.go       -- template rendering
  dropper.constants.go       -- all constants
embed.go                     -- go:embed for versions.yaml, static/*, templates/*
templates/                   -- Go HTML templates
static/                      -- vendored HTMX, CSS, JS
```

---

## API Routes

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/login` | No | Login page |
| POST | `/login` | No | Authenticate with secret |
| POST | `/logout` | Yes | Destroy session |
| GET | `/` | Yes | Main file browser view |
| GET | `/files?path=&sort=&order=` | Yes | File list (HTMX partial) |
| POST | `/files/upload?path=` | Yes | Upload file(s) |
| GET | `/files/download?path=` | Yes | Download file |
| POST | `/files/mkdir?path=&name=` | Yes | Create directory |
| GET | `/healthz` | No | Health check + disk usage |
| GET | `/version` | No | Version info |
| GET | `/metrics` | No | Prometheus metrics |
| GET | `/static/*` | No | Static assets |

---

## Security

- **Path traversal prevention:** Every client-supplied path goes through Clean, Abs, HasPrefix, EvalSymlinks, then re-check. Rejection returns 403 with zero path info in the response.
- **Upload safety:** Temp dir write, then rename into place. Filename sanitized to `[a-zA-Z0-9_.-]`. Extension whitelist checked before any disk write. `http.MaxBytesReader` enforced.
- **Authentication:** `crypto/subtle.ConstantTimeCompare` for secret verification. `crypto/rand` for 32-byte session tokens. Rate-limited login.
- **Security headers:** `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Content-Security-Policy: default-src 'self'`.

---

## License

Apache 2.0 — see [LICENSE](LICENSE) file.
