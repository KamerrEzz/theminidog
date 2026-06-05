package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kamerrezz/theminidog/internal/model"
	"github.com/kamerrezz/theminidog/internal/server/alerting"
	"github.com/kamerrezz/theminidog/internal/server/dashboard"
	"github.com/kamerrezz/theminidog/internal/server/storage"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeMetricRepo implements the dashboard package's metricSource interface
// (Query + Hosts). Note: storage.MetricRepository does not yet have Hosts
// (added in PR3/task 3.1); this fake satisfies the dashboard-local interface
// via structural typing.
type fakeMetricRepo struct {
	hosts  []string
	points []storage.QueryPoint
	err    error
}

func (f *fakeMetricRepo) Query(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
	return f.points, f.err
}

func (f *fakeMetricRepo) Hosts(_ context.Context, _ time.Duration) ([]string, error) {
	return f.hosts, f.err
}

// fakeLogRepo implements storage.LogRepository.
type fakeLogRepo struct {
	entries []storage.LogQueryResult
	err     error
}

func (f *fakeLogRepo) Insert(_ context.Context, _ model.LogBatch) (int, error) {
	return 0, nil
}

func (f *fakeLogRepo) Query(_ context.Context, params storage.LogQueryParams) ([]storage.LogQueryResult, int64, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	// Respect the limit cap so tests can verify clamping at the handler level.
	entries := f.entries
	if params.Limit > 0 && len(entries) > params.Limit {
		entries = entries[:params.Limit]
	}
	return entries, 0, nil
}

func (f *fakeLogRepo) Ping(_ context.Context) error { return nil }

// fakeAlerter implements alerting.AlertReader.
type fakeAlerter struct {
	alerts []alerting.Alert
}

func (f *fakeAlerter) ActiveAlerts() []alerting.Alert {
	return f.alerts
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newHandler(mRepo *fakeMetricRepo, lRepo storage.LogRepository, alerter alerting.AlertReader) *dashboard.DashHandler {
	return dashboard.NewDashHandler(mRepo, lRepo, alerter, nil)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// Test 1: HandleDashboard returns 200 with Content-Type text/html.
func TestHandleDashboard_OK(t *testing.T) {
	h := newHandler(&fakeMetricRepo{}, &fakeLogRepo{}, &fakeAlerter{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.HandleDashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected Content-Type to contain text/html, got %q", ct)
	}
}

// Test 2: HandleDashboardMetrics missing host returns 400.
func TestHandleDashboardMetrics_MissingHost(t *testing.T) {
	h := newHandler(&fakeMetricRepo{}, &fakeLogRepo{}, &fakeAlerter{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/metrics", nil)
	rec := httptest.NewRecorder()
	h.HandleDashboardMetrics(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// Test 3: HandleDashboardMetrics valid host returns 200 with "series" key.
func TestHandleDashboardMetrics_ValidHost(t *testing.T) {
	h := newHandler(&fakeMetricRepo{}, &fakeLogRepo{}, &fakeAlerter{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/metrics?host=web-01", nil)
	rec := httptest.NewRecorder()
	h.HandleDashboardMetrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if _, ok := body["series"]; !ok {
		t.Fatalf("expected JSON key 'series' in response, got keys: %v", keysOf(body))
	}
}

// Test 4: HandleDashboardMetrics invalid window returns 400.
func TestHandleDashboardMetrics_InvalidWindow(t *testing.T) {
	h := newHandler(&fakeMetricRepo{}, &fakeLogRepo{}, &fakeAlerter{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/metrics?host=web-01&window=invalid", nil)
	rec := httptest.NewRecorder()
	h.HandleDashboardMetrics(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// Test 5: HandleDashboardLogs missing host returns 400.
func TestHandleDashboardLogs_MissingHost(t *testing.T) {
	h := newHandler(&fakeMetricRepo{}, &fakeLogRepo{}, &fakeAlerter{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/logs", nil)
	rec := httptest.NewRecorder()
	h.HandleDashboardLogs(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// Test 6: HandleDashboardLogs valid host returns 200 with "entries" key.
func TestHandleDashboardLogs_ValidHost(t *testing.T) {
	h := newHandler(&fakeMetricRepo{}, &fakeLogRepo{}, &fakeAlerter{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/logs?host=web-01", nil)
	rec := httptest.NewRecorder()
	h.HandleDashboardLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if _, ok := body["entries"]; !ok {
		t.Fatalf("expected JSON key 'entries' in response, got keys: %v", keysOf(body))
	}
}

// Test 7: HandleDashboardLogs limit clamped to 200 when limit=999 is passed.
// The fake returns as many entries as the handler requests; we verify the
// handler itself never passes limit > 200 to the repo (checked via a counting fake).
func TestHandleDashboardLogs_LimitClamped(t *testing.T) {
	countingRepo := &countingLogRepo{}
	h := newHandler(&fakeMetricRepo{}, countingRepo, &fakeAlerter{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/logs?host=web-01&limit=999", nil)
	rec := httptest.NewRecorder()
	h.HandleDashboardLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if countingRepo.lastLimit > 200 {
		t.Fatalf("handler passed limit %d to repo; expected <= 200", countingRepo.lastLimit)
	}
}

// countingLogRepo records the limit passed to Query.
type countingLogRepo struct {
	lastLimit int
}

func (c *countingLogRepo) Insert(_ context.Context, _ model.LogBatch) (int, error) { return 0, nil }
func (c *countingLogRepo) Query(_ context.Context, p storage.LogQueryParams) ([]storage.LogQueryResult, int64, error) {
	c.lastLimit = p.Limit
	return []storage.LogQueryResult{}, 0, nil
}
func (c *countingLogRepo) Ping(_ context.Context) error { return nil }

// Test 8: HandleDashboard with a firing alert returns 200 (template renders without error).
func TestHandleDashboard_WithFiringAlert(t *testing.T) {
	alert := alerting.Alert{
		Rule: alerting.Rule{
			Host:      "web-01",
			Name:      "cpu.usage_pct",
			Op:        alerting.OpGT,
			Threshold: 90.0,
			For:       5 * time.Minute,
		},
		State:     alerting.StateFiring,
		UpdatedAt: time.Now(),
		Value:     95.5,
	}
	alerter := &fakeAlerter{alerts: []alerting.Alert{alert}}
	metricRepo := &fakeMetricRepo{hosts: []string{"web-01"}}
	h := newHandler(metricRepo, &fakeLogRepo{}, alerter)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.HandleDashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with firing alert, got %d\nbody: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "web-01") {
		t.Fatalf("expected host 'web-01' in rendered HTML")
	}
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

func keysOf(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
