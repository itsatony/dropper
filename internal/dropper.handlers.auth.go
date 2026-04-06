package dropper

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

// sessionContextKey is the context key used to store the session.
const sessionContextKey contextKey = "session"

// SessionFromContext retrieves the session from the request context.
// Returns nil if no session is present.
func SessionFromContext(ctx context.Context) *Session {
	s, _ := ctx.Value(sessionContextKey).(*Session)
	return s
}

// setSessionCookie sets the session cookie on the response.
func setSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     CookiePath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearSessionCookie sets an expired cookie to clear it from the browser.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     CookiePath,
		MaxAge:   CookieDeleteMaxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

// clientIP extracts the client IP from the request. chi's RealIP middleware
// has already set r.RemoteAddr to the value from X-Real-IP or X-Forwarded-For.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// wantsJSON checks if the client prefers a JSON response.
func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get(HeaderAccept), ContentTypeJSON)
}

// HandleLoginPage returns a handler that renders the login form.
// GET /login — uses the centralized TemplateSet.
func HandleLoginPage(ts *TemplateSet, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := ts.RenderPage(w, PageLogin, http.StatusOK, loginData{}); err != nil {
			logger.Error(ErrMsgTemplateRender, LogFieldError, err)
		}
	}
}

// HandleLogin returns a handler that validates the shared secret, creates a
// session, and sets a cookie. POST /login — uses the centralized TemplateSet.
func HandleLogin(store *SessionStore, configSecret string, limiter *RateLimiter, ts *TemplateSet, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)

		// Rate limit check.
		if !limiter.Allow(ip) {
			logger.Warn(LogMsgRateLimited, LogFieldIP, ip)
			if wantsJSON(r) {
				RespondError(w, http.StatusTooManyRequests, ErrCodeTooManyReqs, ErrMsgRateLimitExceeded)
				return
			}
			if err := ts.RenderPage(w, PageLogin, http.StatusTooManyRequests, loginData{Error: ErrMsgRateLimitExceeded}); err != nil {
				logger.Error(ErrMsgTemplateRender, LogFieldError, err)
			}
			return
		}

		if err := r.ParseForm(); err != nil {
			logger.Warn(LogMsgLoginFailed, LogFieldIP, ip, LogFieldError, err)
			if err := ts.RenderPage(w, PageLogin, http.StatusBadRequest, loginData{Error: ErrMsgInvalidCredential}); err != nil {
				logger.Error(ErrMsgTemplateRender, LogFieldError, err)
			}
			return
		}

		inputSecret := r.FormValue(FormFieldLoginInput)

		// Constant-time comparison to prevent timing attacks.
		if subtle.ConstantTimeCompare([]byte(inputSecret), []byte(configSecret)) != 1 {
			logger.Warn(LogMsgLoginFailed, LogFieldIP, ip)
			if err := ts.RenderPage(w, PageLogin, http.StatusUnauthorized, loginData{Error: ErrMsgInvalidCredential}); err != nil {
				logger.Error(ErrMsgTemplateRender, LogFieldError, err)
			}
			return
		}

		// Create session.
		token, err := store.Create()
		if err != nil {
			logger.Error(ErrMsgTokenGeneration, LogFieldError, err)
			RespondError(w, http.StatusInternalServerError, ErrCodeInternal, ErrMsgTokenGeneration)
			return
		}

		ttlSeconds := int(store.ttl.Seconds())
		setSessionCookie(w, token, ttlSeconds)

		logger.Info(LogMsgLoginSuccess,
			LogFieldIP, ip,
			LogFieldSessionID, sessionTokenPrefix(token),
		)

		http.Redirect(w, r, RouteRoot, http.StatusSeeOther)
	}
}

// HandleLogout returns a handler that destroys the session and clears the
// cookie. POST /logout
func HandleLogout(store *SessionStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookieName)
		if err == nil && cookie.Value != "" {
			logger.Info(LogMsgLogout,
				LogFieldSessionID, sessionTokenPrefix(cookie.Value),
				LogFieldIP, clientIP(r),
			)
			store.Delete(cookie.Value)
		}

		clearSessionCookie(w)
		http.Redirect(w, r, RouteLogin, http.StatusSeeOther)
	}
}

// SessionMiddleware returns middleware that validates session cookies on
// protected routes. On failure it redirects browsers to /login or returns
// 401 JSON for API clients.
func SessionMiddleware(store *SessionStore, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil {
				logger.Debug(LogMsgAuthMiddleware, LogFieldIP, clientIP(r))
				denyAccess(w, r)
				return
			}

			session := store.Get(cookie.Value)
			if session == nil {
				logger.Debug(LogMsgAuthMiddleware, LogFieldIP, clientIP(r))
				clearSessionCookie(w)
				denyAccess(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), sessionContextKey, session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// denyAccess responds to an unauthenticated request: JSON 401 for API clients,
// redirect to /login for browsers.
func denyAccess(w http.ResponseWriter, r *http.Request) {
	if wantsJSON(r) {
		RespondError(w, http.StatusUnauthorized, ErrCodeUnauthorized, ErrMsgSessionNotFound)
		return
	}
	http.Redirect(w, r, RouteLogin, http.StatusSeeOther)
}
