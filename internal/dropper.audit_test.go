package dropper

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func auditTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testAuditLogger(t *testing.T) (*AuditLogger, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.log")
	al, err := NewAuditLogger(path, auditTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = al.Close() })
	return al, path
}

func readAuditLines(t *testing.T, path string) []AuditEntry {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	var entries []AuditEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry AuditEntry
		err := json.Unmarshal(scanner.Bytes(), &entry)
		require.NoError(t, err, "line should be valid JSON: %s", scanner.Text())
		entries = append(entries, entry)
	}
	require.NoError(t, scanner.Err())
	return entries
}

// readRawLines returns the raw JSON lines (as map) so we can check field presence.
func readRawLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	var lines []map[string]any
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var m map[string]any
		err := json.Unmarshal(scanner.Bytes(), &m)
		require.NoError(t, err)
		lines = append(lines, m)
	}
	require.NoError(t, scanner.Err())
	return lines
}

// --- NewAuditLogger tests ---

func TestNewAuditLogger_CreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	al, err := NewAuditLogger(path, auditTestLogger())
	require.NoError(t, err)
	defer al.Close()

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, FilePermissions, info.Mode().Perm())
	assert.True(t, al.enabled)
}

func TestNewAuditLogger_DisabledEmptyPath(t *testing.T) {
	al, err := NewAuditLogger("", auditTestLogger())
	require.NoError(t, err)
	assert.False(t, al.enabled)
	assert.Nil(t, al.file)
}

func TestNewAuditLogger_InvalidPath(t *testing.T) {
	al, err := NewAuditLogger("/nonexistent/dir/audit.log", auditTestLogger())
	assert.Nil(t, al)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrMsgAuditOpen)
}

// --- Log tests ---

func TestAuditLogger_Log_WritesValidJSON(t *testing.T) {
	al, path := testAuditLogger(t)

	al.Log(AuditEntry{
		ClientIP: "10.0.0.1",
		Action:   AuditActionUpload,
		Path:     "/files/test.txt",
		Success:  true,
	})

	entries := readAuditLines(t, path)
	require.Len(t, entries, 1)
	assert.Equal(t, "10.0.0.1", entries[0].ClientIP)
	assert.Equal(t, AuditActionUpload, entries[0].Action)
	assert.Equal(t, "/files/test.txt", entries[0].Path)
	assert.True(t, entries[0].Success)
}

func TestAuditLogger_Log_AllFields(t *testing.T) {
	al, path := testAuditLogger(t)

	size := int64(1024)
	al.Log(AuditEntry{
		ClientIP: "192.168.1.1",
		Action:   AuditActionUpload,
		Path:     "/docs/report.pdf",
		FileSize: &size,
		Success:  false,
		Error:    ErrMsgExtNotAllowed,
	})

	raw := readRawLines(t, path)
	require.Len(t, raw, 1)

	assert.Contains(t, raw[0], "timestamp")
	assert.Contains(t, raw[0], "client_ip")
	assert.Contains(t, raw[0], "action")
	assert.Contains(t, raw[0], "path")
	assert.Contains(t, raw[0], "file_size")
	assert.Contains(t, raw[0], "success")
	assert.Contains(t, raw[0], "error")
}

func TestAuditLogger_Log_OmitsNilFileSize(t *testing.T) {
	al, path := testAuditLogger(t)

	al.Log(AuditEntry{
		ClientIP: "10.0.0.1",
		Action:   AuditActionDownload,
		Path:     "/files/test.txt",
		FileSize: nil,
		Success:  true,
	})

	raw := readRawLines(t, path)
	require.Len(t, raw, 1)
	_, hasFileSize := raw[0]["file_size"]
	assert.False(t, hasFileSize, "file_size should be omitted when nil")
}

func TestAuditLogger_Log_IncludesZeroFileSize(t *testing.T) {
	al, path := testAuditLogger(t)

	size := int64(0)
	al.Log(AuditEntry{
		ClientIP: "10.0.0.1",
		Action:   AuditActionUpload,
		Path:     "/files/empty.txt",
		FileSize: &size,
		Success:  true,
	})

	raw := readRawLines(t, path)
	require.Len(t, raw, 1)
	fileSize, hasFileSize := raw[0]["file_size"]
	assert.True(t, hasFileSize, "file_size should be present for zero-byte file")
	assert.Equal(t, float64(0), fileSize) // JSON numbers decode as float64
}

func TestAuditLogger_Log_OmitsEmptyError(t *testing.T) {
	al, path := testAuditLogger(t)

	al.Log(AuditEntry{
		ClientIP: "10.0.0.1",
		Action:   AuditActionMkdir,
		Path:     "/files/newdir",
		Success:  true,
		Error:    "",
	})

	raw := readRawLines(t, path)
	require.Len(t, raw, 1)
	_, hasError := raw[0]["error"]
	assert.False(t, hasError, "error should be omitted when empty")
}

func TestAuditLogger_Log_DisabledNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger("", auditTestLogger())
	require.NoError(t, err)

	al.Log(AuditEntry{
		ClientIP: "10.0.0.1",
		Action:   AuditActionUpload,
		Path:     "/files/test.txt",
		Success:  true,
	})

	// File should not exist since logger was disabled.
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err))
}

func TestAuditLogger_Log_SetsTimestamp(t *testing.T) {
	al, path := testAuditLogger(t)

	before := time.Now().UTC()
	al.Log(AuditEntry{
		ClientIP: "10.0.0.1",
		Action:   AuditActionUpload,
		Path:     "/test.txt",
		Success:  true,
	})
	after := time.Now().UTC()

	entries := readAuditLines(t, path)
	require.Len(t, entries, 1)
	assert.NotEmpty(t, entries[0].Timestamp)

	ts, err := time.Parse(AuditTimestampFormat, entries[0].Timestamp)
	require.NoError(t, err, "timestamp should parse as RFC3339Nano")
	assert.True(t, !ts.Before(before.Truncate(time.Microsecond)), "timestamp should be >= test start")
	assert.True(t, !ts.After(after.Add(time.Second)), "timestamp should be <= test end")
}

func TestAuditLogger_Log_Concurrent(t *testing.T) {
	al, path := testAuditLogger(t)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(n int) {
			defer wg.Done()
			al.Log(AuditEntry{
				ClientIP: "10.0.0.1",
				Action:   AuditActionUpload,
				Path:     "/concurrent/test.txt",
				Success:  true,
			})
		}(i)
	}

	wg.Wait()

	entries := readAuditLines(t, path)
	assert.Len(t, entries, goroutines, "should have exactly %d entries", goroutines)
}

func TestAuditLogger_Log_MultipleEntries(t *testing.T) {
	al, path := testAuditLogger(t)

	const count = 100
	for i := range count {
		size := int64(i * 1024)
		al.Log(AuditEntry{
			ClientIP: "10.0.0.1",
			Action:   AuditActionUpload,
			Path:     "/batch/test.txt",
			FileSize: &size,
			Success:  true,
		})
	}

	entries := readAuditLines(t, path)
	assert.Len(t, entries, count)

	// Verify each entry is independently valid.
	for idx, entry := range entries {
		assert.NotEmpty(t, entry.Timestamp, "entry %d should have timestamp", idx)
		assert.Equal(t, AuditActionUpload, entry.Action, "entry %d action", idx)
		require.NotNil(t, entry.FileSize, "entry %d should have file_size", idx)
		assert.Equal(t, int64(idx*1024), *entry.FileSize, "entry %d file_size", idx)
	}
}

// --- Close tests ---

func TestAuditLogger_Close(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	al, err := NewAuditLogger(path, auditTestLogger())
	require.NoError(t, err)

	err = al.Close()
	assert.NoError(t, err)
}

func TestAuditLogger_Log_AfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	al, err := NewAuditLogger(path, auditTestLogger())
	require.NoError(t, err)

	al.Log(AuditEntry{
		ClientIP: "10.0.0.1",
		Action:   AuditActionUpload,
		Path:     "/before-close.txt",
		Success:  true,
	})

	require.NoError(t, al.Close())

	// Log after Close must not panic — should be a silent no-op.
	assert.NotPanics(t, func() {
		al.Log(AuditEntry{
			ClientIP: "10.0.0.2",
			Action:   AuditActionDownload,
			Path:     "/after-close.txt",
			Success:  true,
		})
	})

	// Only the pre-close entry should be in the file.
	entries := readAuditLines(t, path)
	assert.Len(t, entries, 1)
	assert.Equal(t, "/before-close.txt", entries[0].Path)
}

func TestAuditLogger_Close_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	al, err := NewAuditLogger(path, auditTestLogger())
	require.NoError(t, err)

	assert.NoError(t, al.Close())
	assert.NotPanics(t, func() {
		_ = al.Close()
	})
}

// --- Reopen tests ---

func TestAuditLogger_Reopen(t *testing.T) {
	al, path := testAuditLogger(t)

	// Write before reopen.
	al.Log(AuditEntry{
		ClientIP: "10.0.0.1",
		Action:   AuditActionUpload,
		Path:     "/before.txt",
		Success:  true,
	})

	require.NoError(t, al.Reopen())

	// Write after reopen.
	al.Log(AuditEntry{
		ClientIP: "10.0.0.2",
		Action:   AuditActionDownload,
		Path:     "/after.txt",
		Success:  true,
	})

	entries := readAuditLines(t, path)
	require.Len(t, entries, 2)
	assert.Equal(t, "/before.txt", entries[0].Path)
	assert.Equal(t, "/after.txt", entries[1].Path)
}

func TestAuditLogger_Reopen_AfterDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	al, err := NewAuditLogger(path, auditTestLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = al.Close() })

	// Write, then delete the file (simulating logrotate mv).
	al.Log(AuditEntry{
		ClientIP: "10.0.0.1",
		Action:   AuditActionUpload,
		Path:     "/pre-rotate.txt",
		Success:  true,
	})

	require.NoError(t, os.Remove(path))

	// Reopen creates a new file.
	require.NoError(t, al.Reopen())

	al.Log(AuditEntry{
		ClientIP: "10.0.0.2",
		Action:   AuditActionDownload,
		Path:     "/post-rotate.txt",
		Success:  true,
	})

	// New file should have only the post-rotation entry.
	entries := readAuditLines(t, path)
	require.Len(t, entries, 1)
	assert.Equal(t, "/post-rotate.txt", entries[0].Path)
}

func TestAuditLogger_Reopen_FailurePath(t *testing.T) {
	al, _ := testAuditLogger(t)

	// Mutate path to an invalid directory to force OpenFile failure on Reopen.
	al.path = "/nonexistent/dir/audit.log"

	err := al.Reopen()
	require.Error(t, err)
	assert.Contains(t, err.Error(), ErrMsgAuditOpen)

	// Logger should be disabled after failed reopen — Log must not panic.
	assert.NotPanics(t, func() {
		al.Log(AuditEntry{
			ClientIP: "10.0.0.1",
			Action:   AuditActionUpload,
			Path:     "/should-not-write.txt",
			Success:  true,
		})
	})
}

// --- NewAuditEntry tests ---

func TestNewAuditEntry_SetsFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/files/test.txt", nil)
	req.RemoteAddr = "10.0.0.5:54321"

	entry := NewAuditEntry(req, AuditActionDownload, "/files/test.txt")

	assert.Equal(t, "10.0.0.5", entry.ClientIP)
	assert.Equal(t, AuditActionDownload, entry.Action)
	assert.Equal(t, "/files/test.txt", entry.Path)
	assert.Empty(t, entry.Timestamp, "timestamp should be empty (set by Log)")
	assert.False(t, entry.Success, "success should default to false")
	assert.Nil(t, entry.FileSize, "file_size should default to nil")
}

// --- DC-09 heavy concurrent writes ---

func TestAuditLogger_ConcurrentWrites_Heavy(t *testing.T) {
	al, path := testAuditLogger(t)

	const goroutines = 10
	const entriesPerGoroutine = 100
	const totalEntries = goroutines * entriesPerGoroutine

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := range goroutines {
		go func(gID int) {
			defer wg.Done()
			for i := range entriesPerGoroutine {
				size := int64(gID*entriesPerGoroutine + i)
				al.Log(AuditEntry{
					ClientIP: "10.0.0.1",
					Action:   AuditActionUpload,
					Path:     "/heavy/test.txt",
					FileSize: &size,
					Success:  true,
				})
			}
		}(g)
	}

	wg.Wait()

	entries := readAuditLines(t, path)
	assert.Len(t, entries, totalEntries, "should have exactly %d entries", totalEntries)

	// Every entry must be valid JSON with expected fields.
	for idx, entry := range entries {
		assert.NotEmpty(t, entry.Timestamp, "entry %d should have timestamp", idx)
		assert.Equal(t, AuditActionUpload, entry.Action, "entry %d action", idx)
		assert.True(t, entry.Success, "entry %d success", idx)
	}
}

func TestNewAuditEntry_ClientIPWithPort(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		expected   string
	}{
		{"ip4 with port", "192.168.1.1:8080", "192.168.1.1"},
		{"ip4 without port", "192.168.1.1", "192.168.1.1"},
		{"ip6 with port", "[::1]:8080", "::1"},
		{"ip6 without port", "::1", "::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			entry := NewAuditEntry(req, AuditActionUpload, "/test")
			assert.Equal(t, tt.expected, entry.ClientIP)
		})
	}
}
