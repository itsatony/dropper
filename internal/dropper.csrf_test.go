package dropper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// csrfTestHandler is a simple handler that returns 200 OK when reached.
var csrfTestHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestCSRFMiddleware_SafeMethodsBypass(t *testing.T) {
	logger := testLogger()
	middleware := CSRFMiddleware(logger)(csrfTestHandler)

	methods := []string{MethodGet, MethodHead, MethodOptions}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/", nil)
			// Even with a mismatched origin, safe methods pass through.
			req.Header.Set(HeaderOrigin, "https://evil.example.com")
			rec := httptest.NewRecorder()

			middleware.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func TestCSRFMiddleware_PostWithMatchingOrigin(t *testing.T) {
	logger := testLogger()
	middleware := CSRFMiddleware(logger)(csrfTestHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = "localhost:8080"
	req.Header.Set(HeaderOrigin, "http://localhost:8080")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRFMiddleware_PostWithMatchingOriginHTTPS(t *testing.T) {
	logger := testLogger()
	middleware := CSRFMiddleware(logger)(csrfTestHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = "drop.example.com"
	req.Header.Set(HeaderOrigin, "https://drop.example.com")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRFMiddleware_PostWithMatchingReferer(t *testing.T) {
	logger := testLogger()
	middleware := CSRFMiddleware(logger)(csrfTestHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = "localhost:8080"
	// No Origin, but valid Referer.
	req.Header.Set(HeaderReferer, "http://localhost:8080/some/page")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRFMiddleware_PostWithMismatchedOrigin(t *testing.T) {
	logger := testLogger()
	middleware := CSRFMiddleware(logger)(csrfTestHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = "localhost:8080"
	req.Header.Set(HeaderOrigin, "https://evil.example.com")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRFMiddleware_PostWithMismatchedReferer(t *testing.T) {
	logger := testLogger()
	middleware := CSRFMiddleware(logger)(csrfTestHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = "localhost:8080"
	// No Origin, mismatched Referer.
	req.Header.Set(HeaderReferer, "https://evil.example.com/attack")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRFMiddleware_PostWithNoOriginNoReferer(t *testing.T) {
	logger := testLogger()
	middleware := CSRFMiddleware(logger)(csrfTestHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = "localhost:8080"
	// Neither Origin nor Referer — lenient policy allows this.
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRFMiddleware_OriginTakesPrecedenceOverReferer(t *testing.T) {
	logger := testLogger()
	middleware := CSRFMiddleware(logger)(csrfTestHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = "localhost:8080"
	// Origin matches but Referer does not — Origin wins.
	req.Header.Set(HeaderOrigin, "http://localhost:8080")
	req.Header.Set(HeaderReferer, "https://evil.example.com/page")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRFMiddleware_MismatchedOriginIgnoresReferer(t *testing.T) {
	logger := testLogger()
	middleware := CSRFMiddleware(logger)(csrfTestHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = "localhost:8080"
	// Origin mismatches — rejected even if Referer matches.
	req.Header.Set(HeaderOrigin, "https://evil.example.com")
	req.Header.Set(HeaderReferer, "http://localhost:8080/page")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRFMiddleware_DeleteMethod(t *testing.T) {
	logger := testLogger()
	middleware := CSRFMiddleware(logger)(csrfTestHandler)

	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	req.Host = "localhost:8080"
	req.Header.Set(HeaderOrigin, "https://evil.example.com")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRFMiddleware_PutMethod(t *testing.T) {
	logger := testLogger()
	middleware := CSRFMiddleware(logger)(csrfTestHandler)

	req := httptest.NewRequest(http.MethodPut, "/", nil)
	req.Host = "localhost:8080"
	req.Header.Set(HeaderOrigin, "https://evil.example.com")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRFMiddleware_PatchMethod(t *testing.T) {
	logger := testLogger()
	middleware := CSRFMiddleware(logger)(csrfTestHandler)

	req := httptest.NewRequest(http.MethodPatch, "/", nil)
	req.Host = "localhost:8080"
	req.Header.Set(HeaderOrigin, "https://evil.example.com")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRFMiddleware_PortMismatch(t *testing.T) {
	logger := testLogger()
	middleware := CSRFMiddleware(logger)(csrfTestHandler)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Host = "localhost:8080"
	req.Header.Set(HeaderOrigin, "http://localhost:9090")
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestExtractHostFromURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "http with port",
			input:    "http://localhost:8080",
			expected: "localhost:8080",
		},
		{
			name:     "https without port",
			input:    "https://drop.example.com",
			expected: "drop.example.com",
		},
		{
			name:     "https with port",
			input:    "https://drop.example.com:443",
			expected: "drop.example.com:443",
		},
		{
			name:     "with path",
			input:    "http://localhost:8080/some/path?query=1",
			expected: "localhost:8080",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "just hostname",
			input:    "http://example.com",
			expected: "example.com",
		},
		{
			name:     "IP address with port",
			input:    "http://192.168.1.100:8080",
			expected: "192.168.1.100:8080",
		},
		{
			name:     "malformed URL",
			input:    "://broken",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractHostFromURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSafeMethod(t *testing.T) {
	tests := []struct {
		method string
		safe   bool
	}{
		{MethodGet, true},
		{MethodHead, true},
		{MethodOptions, true},
		{http.MethodPost, false},
		{http.MethodPut, false},
		{http.MethodDelete, false},
		{http.MethodPatch, false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			assert.Equal(t, tt.safe, isSafeMethod(tt.method))
		})
	}
}
