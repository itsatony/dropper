package dropper

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// --- Custom Prometheus metrics ---
// Registered via promauto (auto-registers with prometheus.DefaultRegisterer).
// Spec section 3.8: request count, upload count, upload bytes, error count.

var (
	// RequestsTotal counts all HTTP requests by method, matched route pattern, and status code.
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: MetricsNamespace,
		Name:      MetricNameRequestsTotal,
		Help:      MetricHelpRequests,
	}, []string{MetricLabelMethod, MetricLabelRoute, MetricLabelStatus})

	// UploadsTotal counts successful file uploads.
	UploadsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: MetricsNamespace,
		Name:      MetricNameUploadsTotal,
		Help:      MetricHelpUploads,
	})

	// UploadBytesTotal counts total bytes written by successful uploads.
	UploadBytesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: MetricsNamespace,
		Name:      MetricNameUploadBytes,
		Help:      MetricHelpUploadBytes,
	})

	// ErrorsTotal counts error responses by error code.
	ErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: MetricsNamespace,
		Name:      MetricNameErrorsTotal,
		Help:      MetricHelpErrors,
	}, []string{MetricLabelErrorCode})
)

// MetricsMiddleware records request count with method, route pattern, and status code labels.
// Uses chi's WrapResponseWriter to capture the status code written by downstream handlers.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := chiMiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		routePattern := MetricRouteUnknown
		if rctx := chi.RouteContext(r.Context()); rctx != nil {
			if pattern := rctx.RoutePattern(); pattern != "" {
				routePattern = pattern
			}
		}

		RequestsTotal.WithLabelValues(
			r.Method,
			routePattern,
			strconv.Itoa(ww.Status()),
		).Inc()
	})
}
