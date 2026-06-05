package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/kamerrezz/theminidog/internal/server/storage"
)

// NewRouter builds the chi router with all routes and middleware configured.
// JWT-protected routes live under /api/v1/*; health endpoints are public.
func NewRouter(repo storage.MetricRepository, jwtSecret []byte, reqTimeout time.Duration) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(reqTimeout))

	r.Get("/healthz", HandleHealthz())
	r.Get("/readyz", HandleReadyz(repo))

	r.Group(func(r chi.Router) {
		r.Use(JWTMiddleware(jwtSecret))
		r.Post("/api/v1/metrics", HandleIngest(repo))
		r.Get("/api/v1/metrics/query", HandleQuery(repo))
	})

	return r
}
