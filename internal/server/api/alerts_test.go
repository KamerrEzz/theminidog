package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kamerrezz/theminidog/internal/server/alerting"
	"github.com/kamerrezz/theminidog/internal/server/api"
)

// fakeAlerter is a test double for alerting.AlertReader.
type fakeAlerter struct {
	alerts []alerting.Alert
}

func (f *fakeAlerter) ActiveAlerts() []alerting.Alert {
	return f.alerts
}

func TestHandleAlerts_NilAlerter_ReturnsEmptyArray(t *testing.T) {
	handler := api.ExportedHandleAlerts(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr.Result(), http.StatusOK)

	var body map[string]json.RawMessage
	mustDecode(t, rr.Body, &body)
	raw, ok := body["alerts"]
	if !ok {
		t.Fatal("expected \"alerts\" key in response")
	}
	var alerts []alerting.Alert
	if err := json.Unmarshal(raw, &alerts); err != nil {
		t.Fatalf("unmarshal alerts: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected empty alerts array, got %d items", len(alerts))
	}
}

func TestHandleAlerts_WithFiringAlert_ReturnsCorrectShape(t *testing.T) {
	now := time.Now().UTC()
	rule := alerting.Rule{Host: "web-01", Name: "cpu.usage_pct", Op: alerting.OpGT, Threshold: 90, For: 5 * time.Minute}
	alert := alerting.Alert{Rule: rule, State: alerting.StateFiring, UpdatedAt: now, Value: 95.0}
	fa := &fakeAlerter{alerts: []alerting.Alert{alert}}

	handler := api.ExportedHandleAlerts(fa)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr.Result(), http.StatusOK)

	var body struct {
		Alerts []alerting.Alert `json:"alerts"`
	}
	mustDecode(t, rr.Body, &body)
	if len(body.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(body.Alerts))
	}
	got := body.Alerts[0]
	if got.State != alerting.StateFiring {
		t.Errorf("expected state=firing, got %q", got.State)
	}
	if got.Value != 95.0 {
		t.Errorf("expected value=95.0, got %v", got.Value)
	}
	if got.Rule.Host != "web-01" {
		t.Errorf("expected host=web-01, got %q", got.Rule.Host)
	}
}
