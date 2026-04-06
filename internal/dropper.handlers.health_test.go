package dropper

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func healthTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHandleHealthz_Returns200(t *testing.T) {
	handler := HandleHealthz("/tmp", healthTestLogger())
	req := httptest.NewRequest(http.MethodGet, RouteHealthz, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, ContentTypeJSON, rec.Header().Get(HeaderContentType))

	var resp HealthResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, HealthStatusOK, resp.Status)
}

func TestHandleHealthz_DiskFields(t *testing.T) {
	handler := HandleHealthz("/tmp", healthTestLogger())
	req := httptest.NewRequest(http.MethodGet, RouteHealthz, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var resp HealthResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	require.NotNil(t, resp.Disk, "disk field should be present for valid path")
	assert.Greater(t, resp.Disk.TotalBytes, uint64(0))
	assert.Greater(t, resp.Disk.AvailableBytes, uint64(0))
	assert.GreaterOrEqual(t, resp.Disk.UsedPercent, float64(0))
	assert.LessOrEqual(t, resp.Disk.UsedPercent, DiskPercent100)
}

func TestHandleHealthz_InvalidRootDir(t *testing.T) {
	handler := HandleHealthz("/nonexistent/path/that/does/not/exist", healthTestLogger())
	req := httptest.NewRequest(http.MethodGet, RouteHealthz, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp HealthResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, HealthStatusOK, resp.Status)
	assert.Nil(t, resp.Disk, "disk should be nil for invalid path")
}

// --- DC-10 HTMX disk usage tests ---

func TestHandleHealthz_HTMXResponse_ReturnsHTML(t *testing.T) {
	handler := HandleHealthz("/tmp", healthTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteHealthz, nil)
	req.Header.Set(HeaderHXRequest, HXRequestTrue)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get(HeaderContentType), ContentTypeHTML)

	body := rec.Body.String()
	// Should contain disk usage info rendered as HTML (inner content only, no footer wrapper).
	assert.Contains(t, body, "Disk:")
	assert.Contains(t, body, "disk-bar")
	// Must NOT contain the footer wrapper (prevents HTMX infinite loop).
	assert.NotContains(t, body, "hx-trigger")
	assert.NotContains(t, body, "<footer")
}

func TestHandleHealthz_HTMXResponse_InvalidPath(t *testing.T) {
	handler := HandleHealthz("/nonexistent/path", healthTestLogger())

	req := httptest.NewRequest(http.MethodGet, RouteHealthz, nil)
	req.Header.Set(HeaderHXRequest, HXRequestTrue)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get(HeaderContentType), ContentTypeHTML)
	// Should show unavailable message, not empty body.
	assert.Contains(t, rec.Body.String(), DiskUsageUnavailableHTML)
}

func TestHandleHealthz_JSONPreserved(t *testing.T) {
	// Ensure non-HTMX requests still get JSON.
	handler := HandleHealthz("/tmp", healthTestLogger())
	req := httptest.NewRequest(http.MethodGet, RouteHealthz, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, ContentTypeJSON, rec.Header().Get(HeaderContentType))

	var resp HealthResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, HealthStatusOK, resp.Status)
}

func TestMetricsHandler_Returns200(t *testing.T) {
	handler := MetricsHandler()
	req := httptest.NewRequest(http.MethodGet, RouteMetrics, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, strings.Contains(rec.Body.String(), "go_goroutines"),
		"metrics output should contain default Go metrics")
}
