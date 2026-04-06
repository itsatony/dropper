package dropper

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

// AuditEntry represents a single audit log entry written as one JSON line.
type AuditEntry struct {
	Timestamp string `json:"timestamp"`
	ClientIP  string `json:"client_ip"`
	Action    string `json:"action"`
	Path      string `json:"path"`
	FileSize  *int64 `json:"file_size,omitempty"` // nil = not applicable; 0 = zero-byte file
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

// AuditLogger writes audit entries as JSON lines to an append-only log file.
// It is safe for concurrent use.
type AuditLogger struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	logger  *slog.Logger
	enabled bool
}

// NewAuditLogger creates an AuditLogger that appends JSON lines to the file at
// path. If path is empty, the logger is disabled and all Log calls are no-ops.
func NewAuditLogger(path string, logger *slog.Logger) (*AuditLogger, error) {
	if path == "" {
		logger.Info(LogMsgAuditDisabled)
		return &AuditLogger{logger: logger, enabled: false}, nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, FilePermissions)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMsgAuditOpen, err)
	}

	logger.Info(LogMsgAuditStarted, LogFieldAuditPath, path)

	return &AuditLogger{
		file:    f,
		path:    path,
		logger:  logger,
		enabled: true,
	}, nil
}

// Log writes an audit entry as a single JSON line. If the logger is disabled,
// this is a no-op. The timestamp is auto-set to the current time in RFC3339Nano
// format if not already populated. Errors are logged via slog and never returned
// — audit failures must not break request handling.
func (a *AuditLogger) Log(entry AuditEntry) {
	if !a.enabled {
		return
	}

	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(AuditTimestampFormat)
	}

	data, err := json.Marshal(entry)
	if err != nil {
		a.logger.Error(LogMsgAuditWriteErr, LogFieldError, err)
		return
	}

	data = append(data, '\n')

	a.mu.Lock()
	_, err = a.file.Write(data)
	a.mu.Unlock()

	if err != nil {
		a.logger.Error(LogMsgAuditWriteErr, LogFieldError, err)
	}
}

// NewAuditEntry creates an AuditEntry pre-populated with client IP (extracted
// from the request via chi's RealIP middleware), the given action, and path.
// Timestamp is left empty so that Log fills it at write time.
func NewAuditEntry(r *http.Request, action, path string) AuditEntry {
	return AuditEntry{
		ClientIP: clientIP(r),
		Action:   action,
		Path:     path,
	}
}

// Reopen closes the current audit log file and opens a new handle at the same
// path. This supports log rotation: an external tool renames the file, then
// signals the process to call Reopen, which creates a fresh file.
func (a *AuditLogger) Reopen() error {
	if !a.enabled {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.file.Close(); err != nil {
		return fmt.Errorf("%s: %w", ErrMsgAuditClose, err)
	}

	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, FilePermissions)
	if err != nil {
		return fmt.Errorf("%s: %w", ErrMsgAuditOpen, err)
	}

	a.file = f
	a.logger.Info(LogMsgAuditReopened, LogFieldAuditPath, a.path)
	return nil
}

// Close closes the audit log file. It is safe to call on a disabled logger.
func (a *AuditLogger) Close() error {
	if !a.enabled || a.file == nil {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	err := a.file.Close()
	a.file = nil
	a.logger.Info(LogMsgAuditClosed)

	if err != nil {
		return fmt.Errorf("%s: %w", ErrMsgAuditClose, err)
	}
	return nil
}
