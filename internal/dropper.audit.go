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

// Log writes an audit entry as a single JSON line. If the logger is disabled or
// closed, this is a no-op. The timestamp is auto-set to the current time in
// RFC3339Nano format if not already populated. Errors are logged via slog and
// never returned — audit failures must not break request handling.
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
	if a.file == nil {
		a.mu.Unlock()
		return
	}
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
// File I/O is performed outside the mutex to satisfy the no-mutex-across-I/O rule.
func (a *AuditLogger) Reopen() error {
	a.mu.Lock()
	if !a.enabled {
		a.mu.Unlock()
		return nil
	}
	oldFile := a.file
	a.file = nil // Log() will see nil and no-op during rotation
	a.mu.Unlock()

	// Close old file outside lock.
	if oldFile != nil {
		if err := oldFile.Close(); err != nil {
			return fmt.Errorf("%s: %w", ErrMsgAuditClose, err)
		}
	}

	// Open new file outside lock.
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, FilePermissions)
	if err != nil {
		// Disable logger — cannot write without a valid file handle.
		a.mu.Lock()
		a.enabled = false
		a.mu.Unlock()
		return fmt.Errorf("%s: %w", ErrMsgAuditOpen, err)
	}

	// Swap in new file under lock.
	a.mu.Lock()
	a.file = f
	a.mu.Unlock()

	a.logger.Info(LogMsgAuditReopened, LogFieldAuditPath, a.path)
	return nil
}

// Close closes the audit log file. It is safe to call on a disabled logger or
// after a previous Close call. After Close returns, all subsequent Log calls
// are no-ops. File I/O and logging are performed outside the mutex.
func (a *AuditLogger) Close() error {
	a.mu.Lock()
	if !a.enabled || a.file == nil {
		a.mu.Unlock()
		return nil
	}
	a.enabled = false
	f := a.file
	a.file = nil
	a.mu.Unlock()

	// Close file and log outside the lock.
	a.logger.Info(LogMsgAuditClosed)
	err := f.Close()
	if err != nil {
		return fmt.Errorf("%s: %w", ErrMsgAuditClose, err)
	}
	return nil
}
