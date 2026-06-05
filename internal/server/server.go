package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Server wraps the HTTP server and the database pool for lifecycle management.
type Server struct {
	httpServer *http.Server
	pool       *pgxpool.Pool
	log        *slog.Logger
}

// New creates a Server with the given address, handler, pool, and logger.
func New(addr string, handler http.Handler, pool *pgxpool.Pool, log *slog.Logger) *Server {
	return &Server{
		httpServer: &http.Server{Addr: addr, Handler: handler},
		pool:       pool,
		log:        log,
	}
}

// Start begins listening for requests. Returns http.ErrServerClosed on clean shutdown.
func (s *Server) Start() error {
	s.log.Info("server listening", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully drains in-flight HTTP requests then closes the DB pool.
// HTTP drain happens first so in-flight queries can complete before pool is closed.
func (s *Server) Shutdown(ctx context.Context) {
	s.log.Info("shutting down server")
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.log.Error("server shutdown error", "err", err)
	}
	s.pool.Close()
	s.log.Info("server stopped")
}
