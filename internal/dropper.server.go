package dropper

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/itsatony/go-version"
)

// Server manages the HTTP server lifecycle.
type Server struct {
	httpServer *http.Server
	router     *chi.Mux
	config     *Config
	logger     *slog.Logger
}

// NewServer creates a fully wired server with all routes and middleware.
func NewServer(cfg *Config, logger *slog.Logger, staticFS fs.FS, templateFS fs.FS) (*Server, error) {
	r := chi.NewRouter()

	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(securityHeadersMiddleware)

	// Public routes (no auth required).
	r.Get(RouteHealthz, HandleHealthz(cfg.Dropper.RootDir, logger))
	r.Handle(RouteVersion, version.Handler())
	r.Handle(RouteMetrics, MetricsHandler())

	// Static file server from embedded FS.
	if staticFS != nil {
		staticSub, err := fs.Sub(staticFS, StaticFSPrefix)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ErrMsgStaticFSSub, err)
		}
		fileServer := http.FileServer(http.FS(staticSub))
		r.Handle(RouteStatic, http.StripPrefix(StaticURLPrefix, fileServer))
	}

	addr := net.JoinHostPort("", strconv.Itoa(cfg.Dropper.ListenPort))

	srv := &Server{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      r,
			ReadTimeout:  DefaultReadTimeout,
			WriteTimeout: DefaultWriteTimeout,
			IdleTimeout:  DefaultIdleTimeout,
		},
		router: r,
		config: cfg,
		logger: logger,
	}

	return srv, nil
}

// Start begins listening. Blocks until the server stops.
func (s *Server) Start() error {
	s.logger.Info(LogMsgServerListening, LogFieldAddr, s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown gracefully shuts down the server with the given context deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info(LogMsgShutdownStarted)
	return s.httpServer.Shutdown(ctx)
}

// Router returns the chi router (exposed for testing).
func (s *Server) Router() *chi.Mux {
	return s.router
}

// securityHeadersMiddleware adds security headers to all responses.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(HeaderXContentTypeOpts, ValueNoSniff)
		w.Header().Set(HeaderXFrameOptions, ValueFrameDeny)
		w.Header().Set(HeaderCSP, ValueCSPDefault)
		next.ServeHTTP(w, r)
	})
}
