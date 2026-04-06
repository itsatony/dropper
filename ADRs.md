# Architecture Decision Records

## ADR-001: No SFTP -- Local Filesystem Only

**Date:** 2026-03-30
**Status:** Accepted

**Context:** Initial design considered including an SFTP client to push files to a remote host. This would require SSH key management, credential storage, connection pooling, and a significant security surface.

**Decision:** Dropper operates on its local filesystem only. The "remote" is achieved by mounting the target volume into the container (bind mount, NFS, PVC, etc.).

**Consequences:**
- Zero network file transfer code
- Security surface reduced to path traversal prevention
- Deployment topology determines reach (bind mounts, NFS, PVCs)
- No SSH key management or credential storage

## ADR-002: In-Memory Sessions

**Date:** 2026-03-30
**Status:** Accepted

**Context:** Sessions could be stored in Redis, SQLite, or on disk for persistence across container restarts.

**Decision:** Sessions are stored in an in-memory map with mutex protection. Sessions are lost on restart.

**Consequences:**
- Restart = all users must re-authenticate
- No additional infrastructure dependencies
- Acceptable trade-off for a tool of this scope
- Background cleanup goroutine handles TTL expiration

## ADR-003: No Private Dependencies

**Date:** 2026-03-30
**Status:** Accepted

**Context:** The vAudience ecosystem uses several private Go libraries. Dropper is open-source (Apache 2.0).

**Decision:** All dependencies must be publicly available on GitHub/pkg.go.dev. No private vAudience libraries. Config uses `spf13/viper` + `go-playground/validator` instead of private `vaiconfig`.

**Consequences:**
- Dropper is fully self-contained and buildable by anyone with Go 1.25+
- No private module proxy configuration required
- Version management uses the public `itsatony/go-version` library

## ADR-004: HTMX Over SPA Framework

**Date:** 2026-03-30
**Status:** Accepted

**Context:** The frontend could use React, Vue, or Svelte for richer client-side behavior and a more interactive experience.

**Decision:** Use HTMX with server-rendered Go HTML templates. JavaScript is limited to clipboard paste, drag-drop, localStorage bookmarks, and toast notifications.

**Consequences:**
- Single binary serves everything (API + templates + static assets)
- No frontend build step, no node_modules
- Minimal JavaScript surface area
- Aligns with Go server-side rendering strengths
- Slightly less interactive than a full SPA, but sufficient for the use case
