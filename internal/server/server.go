// Package server provides the HTTP server for the dfgo pipeline API.
package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"dfgo/internal/server/runmgr"
)

// Server is the dfgo HTTP API server.
type Server struct {
	httpServer *http.Server
	manager    *runmgr.RunManager
	addr       string
}

// Config configures the HTTP server.
type Config struct {
	Addr       string
	ManagerCfg runmgr.ManagerConfig
}

// New creates a new Server.
func New(cfg Config) *Server {
	mgr := runmgr.NewRunManager(cfg.ManagerCfg)
	s := &Server{
		manager: mgr,
		addr:    cfg.Addr,
	}

	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpServer = &http.Server{
		Addr:              cfg.Addr,
		Handler:           requestLogger(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s
}

// ListenAndServe starts the server. Blocks until the server shuts down.
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	slog.Info("server listening", "addr", ln.Addr().String())
	return s.httpServer.Serve(ln)
}

// Shutdown gracefully shuts down the server: drains connections, cancels runs.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("shutting down server")
	s.manager.Shutdown(ctx)
	return s.httpServer.Shutdown(ctx)
}

// Manager returns the run manager (for testing).
func (s *Server) Manager() *runmgr.RunManager {
	return s.manager
}

// Handler returns the HTTP handler (for httptest.Server).
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// requestLogger is simple request logging middleware.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Debug("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}
