package dropper

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test helpers ---

const testAuthSecret = "test-secret-for-auth-tests"

func authTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testTemplateFS() fstest.MapFS {
	return fstest.MapFS{
		"templates/layout.html": &fstest.MapFile{
			Data: []byte(`<!DOCTYPE html><html><body>{{block "content" .}}{{end}}</body></html>`),
		},
		"templates/login.html": &fstest.MapFile{
			Data: []byte(`{{define "content"}}<form method="POST" action="/login"><input type="password" name="secret"><button type="submit">Login</button>{{if .Error}}<p class="error">{{.Error}}</p>{{end}}</form>{{end}}`),
		},
	}
}

func testAuthStore(t *testing.T) *SessionStore {
	t.Helper()
	store := NewSessionStore(time.Hour, authTestLogger())
	t.Cleanup(func() { store.Stop() })
	return store
}

func testRateLimiter() *RateLimiter {
	return NewRateLimiter(DefaultRateLimitLogin, RateLimitWindow)
}

func loginFormBody(secret string) io.Reader {
	form := url.Values{}
	form.Set(FormFieldLoginInput, secret)
	return strings.NewReader(form.Encode())
}

// --- HandleLoginPage tests ---

func TestHandleLoginPage_RendersForm(t *testing.T) {
	handler, err := HandleLoginPage(testTemplateFS(), authTestLogger())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, RouteLogin, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get(HeaderContentType), ContentTypeHTML)

	body := rec.Body.String()
	assert.Contains(t, body, "<form")
	assert.Contains(t, body, `type="password"`)
	assert.Contains(t, body, `name="secret"`)
}

func TestHandleLoginPage_NoErrorOnFirstRender(t *testing.T) {
	handler, err := HandleLoginPage(testTemplateFS(), authTestLogger())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, RouteLogin, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	assert.NotContains(t, body, `class="error"`)
}

// --- HandleLogin tests ---

func TestHandleLogin_CorrectSecret(t *testing.T) {
	store := testAuthStore(t)
	handler, err := HandleLogin(store, testAuthSecret, testRateLimiter(), testTemplateFS(), authTestLogger())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, RouteLogin, loginFormBody(testAuthSecret))
	req.Header.Set(HeaderContentType, ContentTypeForm)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should redirect to / with 303.
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, RouteRoot, rec.Header().Get(HeaderLocation))

	// Should set a session cookie.
	cookies := rec.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			sessionCookie = c
			break
		}
	}
	require.NotNil(t, sessionCookie, "session cookie must be set")
	assert.True(t, sessionCookie.HttpOnly, "cookie must be HttpOnly")
	assert.True(t, sessionCookie.Secure, "cookie must be Secure")
	assert.Equal(t, http.SameSiteStrictMode, sessionCookie.SameSite)
	assert.Equal(t, CookiePath, sessionCookie.Path)
	assert.Greater(t, sessionCookie.MaxAge, 0, "cookie MaxAge must be positive")

	// Token must resolve to a valid session.
	session := store.Get(sessionCookie.Value)
	require.NotNil(t, session, "cookie token must map to a valid session")
}

func TestHandleLogin_WrongSecret(t *testing.T) {
	store := testAuthStore(t)
	handler, err := HandleLogin(store, testAuthSecret, testRateLimiter(), testTemplateFS(), authTestLogger())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, RouteLogin, loginFormBody("wrong-secret-value"))
	req.Header.Set(HeaderContentType, ContentTypeForm)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should re-render login page with 401 and error message.
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, ErrMsgInvalidCredential)

	// No session cookie should be set.
	for _, c := range rec.Result().Cookies() {
		assert.NotEqual(t, SessionCookieName, c.Name, "no session cookie on failed login")
	}
}

func TestHandleLogin_EmptySecret(t *testing.T) {
	store := testAuthStore(t)
	handler, err := HandleLogin(store, testAuthSecret, testRateLimiter(), testTemplateFS(), authTestLogger())
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, RouteLogin, loginFormBody(""))
	req.Header.Set(HeaderContentType, ContentTypeForm)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), ErrMsgInvalidCredential)
}

func TestHandleLogin_RateLimited_HTML(t *testing.T) {
	store := testAuthStore(t)
	limiter := NewRateLimiter(DefaultRateLimitLogin, RateLimitWindow)
	handler, err := HandleLogin(store, testAuthSecret, limiter, testTemplateFS(), authTestLogger())
	require.NoError(t, err)

	// Exhaust the rate limit.
	for range DefaultRateLimitLogin {
		req := httptest.NewRequest(http.MethodPost, RouteLogin, loginFormBody("wrong"))
		req.Header.Set(HeaderContentType, ContentTypeForm)
		req.RemoteAddr = "10.0.0.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// Next request should be rate limited.
	req := httptest.NewRequest(http.MethodPost, RouteLogin, loginFormBody("wrong"))
	req.Header.Set(HeaderContentType, ContentTypeForm)
	req.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Contains(t, rec.Body.String(), ErrMsgRateLimitExceeded)
}

func TestHandleLogin_RateLimited_JSON(t *testing.T) {
	store := testAuthStore(t)
	limiter := NewRateLimiter(DefaultRateLimitLogin, RateLimitWindow)
	handler, err := HandleLogin(store, testAuthSecret, limiter, testTemplateFS(), authTestLogger())
	require.NoError(t, err)

	// Exhaust the rate limit.
	for range DefaultRateLimitLogin {
		req := httptest.NewRequest(http.MethodPost, RouteLogin, loginFormBody("wrong"))
		req.Header.Set(HeaderContentType, ContentTypeForm)
		req.RemoteAddr = "10.0.0.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// API request should get JSON 429.
	req := httptest.NewRequest(http.MethodPost, RouteLogin, loginFormBody("wrong"))
	req.Header.Set(HeaderContentType, ContentTypeForm)
	req.Header.Set(HeaderAccept, ContentTypeJSON)
	req.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)

	var errBody ErrorBody
	err = json.NewDecoder(rec.Body).Decode(&errBody)
	require.NoError(t, err)
	assert.Equal(t, ErrCodeTooManyReqs, errBody.Code)
}

func TestHandleLogin_RateLimitRecovers(t *testing.T) {
	store := testAuthStore(t)
	limiter := NewRateLimiter(DefaultRateLimitLogin, 50*time.Millisecond)
	handler, err := HandleLogin(store, testAuthSecret, limiter, testTemplateFS(), authTestLogger())
	require.NoError(t, err)

	// Exhaust rate limit.
	for range DefaultRateLimitLogin {
		req := httptest.NewRequest(http.MethodPost, RouteLogin, loginFormBody("wrong"))
		req.Header.Set(HeaderContentType, ContentTypeForm)
		req.RemoteAddr = "10.0.0.2:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// Wait for window to expire.
	time.Sleep(100 * time.Millisecond)

	// Should succeed now.
	req := httptest.NewRequest(http.MethodPost, RouteLogin, loginFormBody(testAuthSecret))
	req.Header.Set(HeaderContentType, ContentTypeForm)
	req.RemoteAddr = "10.0.0.2:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
}

// --- HandleLogout tests ---

func TestHandleLogout_ValidSession(t *testing.T) {
	store := testAuthStore(t)
	token, err := store.Create()
	require.NoError(t, err)

	handler := HandleLogout(store, authTestLogger())

	req := httptest.NewRequest(http.MethodPost, RouteLogout, nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should redirect to /login.
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, RouteLogin, rec.Header().Get(HeaderLocation))

	// Cookie should be cleared.
	var clearCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookieName {
			clearCookie = c
			break
		}
	}
	require.NotNil(t, clearCookie, "clear cookie must be set")
	assert.Equal(t, CookieDeleteMaxAge, clearCookie.MaxAge)

	// Session must be deleted from store.
	assert.Nil(t, store.Get(token), "session must be deleted after logout")
}

func TestHandleLogout_NoSession(t *testing.T) {
	store := testAuthStore(t)
	handler := HandleLogout(store, authTestLogger())

	req := httptest.NewRequest(http.MethodPost, RouteLogout, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should still redirect gracefully.
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, RouteLogin, rec.Header().Get(HeaderLocation))
}

// --- SessionMiddleware tests ---

func TestSessionMiddleware_ValidSession(t *testing.T) {
	store := testAuthStore(t)
	token, err := store.Create()
	require.NoError(t, err)

	var capturedSession *Session
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSession = SessionFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	middleware := SessionMiddleware(store, authTestLogger())
	handler := middleware(inner)

	req := httptest.NewRequest(http.MethodGet, RouteRoot, nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, capturedSession, "session must be in context")
	assert.Equal(t, token, capturedSession.Token)
}

func TestSessionMiddleware_NoCookie_Browser(t *testing.T) {
	store := testAuthStore(t)
	middleware := SessionMiddleware(store, authTestLogger())
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, RouteRoot, nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Browser should be redirected.
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, RouteLogin, rec.Header().Get(HeaderLocation))
}

func TestSessionMiddleware_NoCookie_JSON(t *testing.T) {
	store := testAuthStore(t)
	middleware := SessionMiddleware(store, authTestLogger())
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, RouteRoot, nil)
	req.Header.Set(HeaderAccept, ContentTypeJSON)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	var errBody ErrorBody
	err := json.NewDecoder(rec.Body).Decode(&errBody)
	require.NoError(t, err)
	assert.Equal(t, ErrCodeUnauthorized, errBody.Code)
}

func TestSessionMiddleware_ExpiredSession(t *testing.T) {
	store := NewSessionStore(50*time.Millisecond, authTestLogger())
	t.Cleanup(func() { store.Stop() })

	token, err := store.Create()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	middleware := SessionMiddleware(store, authTestLogger())
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called for expired session")
	}))

	req := httptest.NewRequest(http.MethodGet, RouteRoot, nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
}

func TestSessionMiddleware_InvalidToken(t *testing.T) {
	store := testAuthStore(t)
	middleware := SessionMiddleware(store, authTestLogger())
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called for invalid token")
	}))

	req := httptest.NewRequest(http.MethodGet, RouteRoot, nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "bogus-token-value"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
}

// --- Helper function tests ---

func TestClientIP_WithPort(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		expected   string
	}{
		{"ipv4 with port", "192.168.1.1:12345", "192.168.1.1"},
		{"ipv4 without port", "192.168.1.1", "192.168.1.1"},
		{"ipv6 with port", "[::1]:12345", "::1"},
		{"ipv6 without port", "::1", "::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			assert.Equal(t, tt.expected, clientIP(req))
		})
	}
}

func TestWantsJSON(t *testing.T) {
	tests := []struct {
		name     string
		accept   string
		expected bool
	}{
		{"json accept", ContentTypeJSON, true},
		{"html accept", "text/html", false},
		{"no accept", "", false},
		{"mixed with json", "text/html, application/json", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.accept != "" {
				req.Header.Set(HeaderAccept, tt.accept)
			}
			assert.Equal(t, tt.expected, wantsJSON(req))
		})
	}
}

func TestSessionFromContext_NoSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	assert.Nil(t, SessionFromContext(req.Context()))
}
