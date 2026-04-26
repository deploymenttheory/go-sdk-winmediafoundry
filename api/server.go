// Package api provides the HTTP REST service for the Windows Update metadata
// platform.
package api

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/deploymenttheory/go-sdk-windowsuup/api/handlers"
	"github.com/deploymenttheory/go-sdk-windowsuup/api/middleware"
	"github.com/deploymenttheory/go-sdk-windowsuup/winupdate"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// ServerConfig holds configuration for the HTTP server.
type ServerConfig struct {
	// Addr is the listen address, e.g. ":8443".
	Addr string
	// TLSConfig is the TLS configuration. Must include client CA and
	// require client certificates for mTLS.
	TLSConfig *tls.Config
}

// Server is the HTTP REST API server.
type Server struct {
	cfg    ServerConfig
	router chi.Router
	logger *zap.Logger
	srv    *http.Server
}

// New creates a Server, wires all routes and middleware, and is ready to serve.
func New(cfg ServerConfig, svc *winupdate.Service, logger *zap.Logger) *Server {
	r := chi.NewRouter()

	// Global middleware (applied to all routes).
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.RequestID)
	r.Use(middleware.Logging(logger))

	// Health endpoints — no mTLS required.
	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(svc))

	// OpenAPI spec — no mTLS required.
	r.Get("/openapi.yaml", serveOpenAPIYAML)
	r.Get("/openapi.json", serveOpenAPIJSON)

	// All /v1/ endpoints require a verified client certificate.
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireClientCert)

		builds := handlers.NewBuildsHandler(svc)
		files := handlers.NewFilesHandler(svc)
		download := handlers.NewDownloadHandler(svc)
		updates := handlers.NewUpdatesHandler(svc)
		diff := handlers.NewDiffHandler(svc)
		feed := handlers.NewFeedHandler(svc, svc.Emitter().Subscribe)

		r.Route("/v1", func(r chi.Router) {
			// Builds.
			r.Get("/builds", builds.List)
			r.Get("/builds/{uuid}", builds.Get)

			// Files.
			r.Get("/builds/{uuid}/files", files.List)

			// Streaming download proxy.
			r.Get("/builds/{uuid}/files/{filename}/download", download.Stream)

			// Live Windows Update fetch.
			r.Post("/updates/fetch", updates.Fetch)

			// Build diff.
			r.Get("/diff", diff.Compare)

			// Change feed.
			r.Get("/feed", feed.List)
			r.Get("/feed/stream", feed.Stream)
		})
	})

	s := &Server{
		cfg:    cfg,
		router: r,
		logger: logger,
	}
	s.srv = &http.Server{
		Addr:         cfg.Addr,
		Handler:      r,
		TLSConfig:    cfg.TLSConfig,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // disabled — streaming downloads may be large
		IdleTimeout:  120 * time.Second,
	}
	return s
}

// Start begins serving and blocks until ctx is cancelled or the server fails.
func (s *Server) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.srv.Shutdown(shutCtx); err != nil {
			s.logger.Error("server shutdown error", zap.Error(err))
		}
	}()

	if s.cfg.TLSConfig != nil {
		s.logger.Info("starting HTTPS server", zap.String("addr", s.cfg.Addr))
		if err := s.srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			return err
		}
	} else {
		s.logger.Warn("starting HTTP server (no TLS — mTLS disabled)", zap.String("addr", s.cfg.Addr))
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return err
		}
	}
	return nil
}

// Handler returns the underlying http.Handler (useful for testing).
func (s *Server) Handler() http.Handler { return s.router }

// ─── Simple built-in handlers ──────────────────────────────────────────────

func healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func readyz(svc *winupdate.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.Store().Ping(r.Context()); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unavailable"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}

func serveOpenAPIYAML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(openAPISpec)
}

func serveOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// Serve the same YAML as-is; clients can parse it.
	// A full JSON conversion is out of scope for this implementation.
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(openAPISpec)
}
