package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/kamerrezz/theminidog/internal/server/alerting"
	"github.com/kamerrezz/theminidog/internal/server/storage"
)

//go:embed templates/dashboard.html
var templateFS embed.FS

// tmpl is parsed at package init — malformed template panics at startup (intentional, NFR-D1).
var tmpl = template.Must(template.ParseFS(templateFS, "templates/dashboard.html"))

// canonicalNames lists all 9 metric names the dashboard renders.
// Mirrors storage.validMetricNames (unexported) — G3 design decision: hardcoded here to keep
// storage stable. Keep in sync when new canonical metric names are added.
var canonicalNames = []string{
	"cpu.usage_pct",
	"mem.used_pct", "mem.used_bytes", "mem.total_bytes",
	"disk.used_pct", "disk.used_bytes", "disk.total_bytes",
	"net.bytes_in", "net.bytes_out",
}

var windowDurations = map[string]time.Duration{
	"5m":  5 * time.Minute,
	"15m": 15 * time.Minute,
	"30m": 30 * time.Minute,
	"1h":  time.Hour,
	"6h":  6 * time.Hour,
}

type pageData struct {
	Hosts        []string
	HostStatuses []storage.HostStatus
	Alerts       []alerting.Alert
	Refresh      int
}

// metricSource is the minimal metric interface the dashboard needs.
// Declared at point of use (same pattern as alerting.MetricQuerier) so that PR2
// compiles before storage.MetricRepository gains the Hosts method in PR3.
// When PR3 adds Hosts to storage.MetricRepository, the concrete pgxMetricRepository
// will satisfy this interface via Go's structural typing — no cast required.
type metricSource interface {
	Query(ctx context.Context, params storage.QueryParams) ([]storage.QueryPoint, error)
	Hosts(ctx context.Context, window time.Duration) ([]string, error)
}

// DashHandler serves the dashboard page and its data API endpoints.
type DashHandler struct {
	metricRepo metricSource
	logRepo    storage.LogRepository
	alerter    alerting.AlertReader
	tracker    *storage.HostTracker
}

// NewDashHandler constructs a DashHandler.
// m must implement metricSource (Query + Hosts); this is satisfied by the concrete
// pgxMetricRepository once PR3 adds Hosts to storage.MetricRepository.
// alerter may be a typed-nil *alerting.Evaluator — ActiveAlerts() is nil-safe.
// tracker may be nil — host status dots are omitted when nil.
func NewDashHandler(m metricSource, l storage.LogRepository, a alerting.AlertReader, tracker *storage.HostTracker) *DashHandler {
	return &DashHandler{metricRepo: m, logRepo: l, alerter: a, tracker: tracker}
}

// HandleDashboard renders the full dashboard HTML page (GET /).
func (d *DashHandler) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hosts, _ := d.metricRepo.Hosts(ctx, 30*time.Minute) // best-effort; empty on error
	if hosts == nil {
		hosts = []string{}
	}
	var alerts []alerting.Alert
	if d.alerter != nil {
		alerts = d.alerter.ActiveAlerts()
	}
	if alerts == nil {
		alerts = []alerting.Alert{}
	}
	// Populate host statuses from the tracker (nil-safe; falls back to empty slice).
	hostStatuses := d.tracker.All()
	if hostStatuses == nil {
		hostStatuses = []storage.HostStatus{}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, pageData{
		Hosts:        hosts,
		HostStatuses: hostStatuses,
		Alerts:       alerts,
		Refresh:      30,
	}); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// metricSeries is one named time-series in the dashboard metrics response.
type metricSeries struct {
	Name   string               `json:"name"`
	Points []storage.QueryPoint `json:"points"`
}

// dashMetricsResponse is the JSON envelope for GET /api/v1/dashboard/metrics.
type dashMetricsResponse struct {
	Host   string         `json:"host"`
	Window string         `json:"window"`
	Series []metricSeries `json:"series"`
}

// HandleDashboardMetrics serves GET /api/v1/dashboard/metrics (PUBLIC).
//
// Required query params: host
// Optional: window (5m|15m|30m|1h|6h, default 30m)
// Response: {"host":X,"window":W,"series":[{"name":N,"points":[{time,value}...]}...]}
func (d *DashHandler) HandleDashboardMetrics(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	if host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host is required"})
		return
	}
	windowStr := r.URL.Query().Get("window")
	if windowStr == "" {
		windowStr = "30m"
	}
	window, ok := windowDurations[windowStr]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid window: must be one of 5m,15m,30m,1h,6h"})
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()
	series := make([]metricSeries, 0, len(canonicalNames))
	for _, name := range canonicalNames {
		points, err := d.metricRepo.Query(ctx, storage.QueryParams{
			Host:   host,
			Name:   name,
			From:   now.Add(-window),
			To:     now,
			Bucket: "1m",
			Agg:    "avg",
		})
		if err != nil || points == nil {
			points = []storage.QueryPoint{}
		}
		series = append(series, metricSeries{Name: name, Points: points})
	}
	writeJSON(w, http.StatusOK, dashMetricsResponse{Host: host, Window: windowStr, Series: series})
}

// HandleDashboardLogs serves GET /api/v1/dashboard/logs (PUBLIC).
//
// Required query params: host
// Optional: limit (default 50, max 200)
// Response: {"entries":[...],"next_cursor":null}
func (d *DashHandler) HandleDashboardLogs(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	if host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "host is required"})
		return
	}
	limit := 50
	if ls := r.URL.Query().Get("limit"); ls != "" {
		if n, err := strconv.Atoi(ls); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}

	ctx := r.Context()
	entries, _, err := d.logRepo.Query(ctx, storage.LogQueryParams{
		Host:  host,
		Limit: limit,
	})
	if err != nil || entries == nil {
		entries = []storage.LogQueryResult{}
	}
	type logsResp struct {
		Entries    []storage.LogQueryResult `json:"entries"`
		NextCursor *int64                   `json:"next_cursor"`
	}
	writeJSON(w, http.StatusOK, logsResp{Entries: entries})
}

// writeJSON is a local helper that sets Content-Type, writes the status code,
// and JSON-encodes v. Mirrors the api package's writeError style without
// creating an import cycle (dashboard must not import api).
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
