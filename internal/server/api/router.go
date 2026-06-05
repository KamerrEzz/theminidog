package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/kamerrezz/theminidog/internal/server/alerting"
	"github.com/kamerrezz/theminidog/internal/server/dashboard"
	"github.com/kamerrezz/theminidog/internal/server/storage"
)

// NewRouter builds the chi router with all routes and middleware configured.
// JWT-protected routes live under /api/v1/*; health and dashboard endpoints are public.
//
// dash may be nil — if so, GET /, /api/v1/dashboard/metrics, /api/v1/dashboard/logs are not registered.
// alerter may be nil — if so, GET /api/v1/alerts is not registered.
func NewRouter(
	metricRepo storage.MetricRepository,
	logRepo storage.LogRepository,
	jwtSecret []byte,
	reqTimeout time.Duration,
	dash *dashboard.DashHandler,
	alerter alerting.AlertReader,
) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(reqTimeout))

	r.Get("/healthz", HandleHealthz())
	r.Get("/readyz", HandleReadyz(metricRepo))

	// Public routes — no JWT required (dashboard page, dashboard APIs, alerts).
	if dash != nil {
		r.Get("/", dash.HandleDashboard)
		r.Get("/api/v1/dashboard/metrics", dash.HandleDashboardMetrics)
		r.Get("/api/v1/dashboard/logs", dash.HandleDashboardLogs)
	}
	if alerter != nil {
		r.Get("/api/v1/alerts", handleAlerts(alerter))
	}

	r.Group(func(r chi.Router) {
		r.Use(JWTMiddleware(jwtSecret))
		r.Post("/api/v1/metrics", HandleIngest(metricRepo))
		r.Get("/api/v1/metrics/query", HandleQuery(metricRepo))
		r.Post("/api/v1/logs", HandleIngestLogs(logRepo))
		r.Get("/api/v1/logs/query", HandleQueryLogs(logRepo))
	})

	return r
}
