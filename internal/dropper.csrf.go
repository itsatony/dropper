package dropper

import (
	"log/slog"
	"net/http"
	"net/url"
)

// CSRFMiddleware validates that the Origin or Referer header (when present) matches
// the request's Host header on state-changing HTTP methods (POST, PUT, DELETE, PATCH).
//
// Defense-in-depth: SameSite=Strict on session cookies is the primary CSRF defense.
// This middleware adds Origin validation per OWASP's "Verifying Origin with Standard
// Headers" recommendation. If neither Origin nor Referer is present, the request is
// allowed — this accommodates CLI tools (curl, API clients) and privacy proxies that
// strip these headers.
//
// Only the host:port portion is compared (scheme-agnostic) to handle mixed http/https
// when behind a TLS-terminating reverse proxy.
func CSRFMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get(HeaderOrigin)
			referer := r.Header.Get(HeaderReferer)

			// Lenient: if neither header is present, allow the request.
			// Browsers always send Origin on POST; absence indicates a non-browser client.
			if origin == "" && referer == "" {
				next.ServeHTTP(w, r)
				return
			}

			expectedHost := r.Host

			// Check Origin first (preferred — simpler, no path leak).
			if origin != "" {
				originHost := extractHostFromURL(origin)
				if originHost == expectedHost {
					next.ServeHTTP(w, r)
					return
				}

				logger.Warn(LogMsgCSRFRejected,
					LogFieldOrigin, origin,
					LogFieldExpectedHost, expectedHost,
					LogFieldIP, clientIP(r),
				)
				de := NewCSRFError()
				RespondError(w, de.StatusCode, de.Code, de.SafeMsg)
				return
			}

			// Fall back to Referer (some browsers omit Origin on same-origin form POSTs).
			refererHost := extractHostFromURL(referer)
			if refererHost == expectedHost {
				next.ServeHTTP(w, r)
				return
			}

			logger.Warn(LogMsgCSRFRejected,
				LogFieldReferer, referer,
				LogFieldExpectedHost, expectedHost,
				LogFieldIP, clientIP(r),
			)
			de := NewCSRFError()
			RespondError(w, de.StatusCode, de.Code, de.SafeMsg)
		})
	}
}

// isSafeMethod returns true for HTTP methods that do not change server state.
func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

// extractHostFromURL parses a URL string and returns the host:port component.
// For URLs without an explicit port, returns just the hostname.
// Returns an empty string for malformed input.
func extractHostFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}
