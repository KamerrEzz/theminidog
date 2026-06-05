package api

import (
	"encoding/json"
	"net/http"

	"github.com/kamerrezz/theminidog/internal/server/alerting"
)

// handleAlerts returns an http.HandlerFunc that serves GET /api/v1/alerts.
// The endpoint is public (no JWT required) and returns all active alert states.
// When a is nil or no alerts are firing, the response is {"alerts":[]}.
func handleAlerts(a alerting.AlertReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var alerts []alerting.Alert
		if a != nil {
			alerts = a.ActiveAlerts()
		}
		if alerts == nil {
			alerts = []alerting.Alert{}
		}
		type resp struct {
			Alerts []alerting.Alert `json:"alerts"`
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp{Alerts: alerts}) //nolint:errcheck
	}
}
