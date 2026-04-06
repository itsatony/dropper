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
	httpServer   *http.Server
	router       *chi.Mux
	config       *Config
	logger       *slog.Logger
	sessionStore *SessionStore
	auditLogger  *AuditLogger
}

// NewServer creates a fully wired server with all routes and middleware.
func NewServer(cfg *Config, logger *slog.Logger, staticFS fs.FS, templateFS fs.FS) (*Server, error) {
	r := chi.NewRouter()

	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(MetricsMiddleware)
	r.Use(securityHeadersMiddleware)

	// Create audit logger.
	auditLogger, err := NewAuditLogger(cfg.Dropper.AuditLogPath, logger)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMsgAuditInit, err)
	}

	// Create session store and rate limiter.
	ttl, err := cfg.Dropper.SessionTTLDuration()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrMsgConfigValidation, err)
	}
	sessionStore := NewSessionStore(ttl, logger)
	rateLimiter := NewRateLimiter(cfg.Dropper.RateLimitLogin, RateLimitWindow)

	// Create template set from embedded filesystem.
	var ts *TemplateSet
	if templateFS != nil {
		ts, err = NewTemplateSet(templateFS)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ErrMsgTemplateSet, err)
		}
	}

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

	// Auth routes (public).
	if ts != nil {
		r.Get(RouteLogin, HandleLoginPage(ts, logger))
		r.Post(RouteLogin, HandleLogin(sessionStore, cfg.Dropper.Secret, rateLimiter, ts, logger))
	}

	// Protected routes (session required).
	r.Group(func(r chi.Router) {
		r.Use(SessionMiddleware(sessionStore, logger))
		r.Post(RouteLogout, HandleLogout(sessionStore, logger))

		if ts != nil {
			r.Get(RouteRoot, HandleMainPage(ts, &cfg.Dropper, logger))
			r.Get(RouteFiles, HandleListFiles(ts, &cfg.Dropper, logger))
		}

		// File operation routes (JSON-only, no template dependency).
		r.Post(RouteFilesUpload, HandleUpload(&cfg.Dropper, auditLogger, logger))
		r.Get(RouteFilesDownload, HandleDownload(&cfg.Dropper, auditLogger, logger))
		r.Post(RouteFilesMkdir, HandleMkdir(&cfg.Dropper, auditLogger, logger))
	})

	addr := net.JoinHostPort("", strconv.Itoa(cfg.Dropper.ListenPort))

	srv := &Server{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      r,
			ReadTimeout:  DefaultReadTimeout,
			WriteTimeout: DefaultWriteTimeout,
			IdleTimeout:  DefaultIdleTimeout,
		},
		router:       r,
		config:       cfg,
		logger:       logger,
		sessionStore: sessionStore,
		auditLogger:  auditLogger,
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
// Shutdown order: stop session cleanup → drain HTTP connections → close audit log.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info(LogMsgShutdownStarted)
	s.sessionStore.Stop()
	err := s.httpServer.Shutdown(ctx)
	if closeErr := s.auditLogger.Close(); closeErr != nil {
		s.logger.Error(ErrMsgAuditClose, LogFieldError, closeErr)
	}
	return err
}

// Router returns the chi router (exposed for testing).
func (s *Server) Router() *chi.Mux {
	return s.router
}

// AuditLogger returns the server's audit logger for use by handlers.
func (s *Server) AuditLogger() *AuditLogger {
	return s.auditLogger
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
