package api

import (
	"net/http"

	"github.com/kamerrezz/theminidog/internal/server/alerting"
	"github.com/kamerrezz/theminidog/internal/server/storage"
)

// ExportedHandleAlerts exposes the unexported handleAlerts function for
// black-box tests in the api_test package.
func ExportedHandleAlerts(a alerting.AlertReader) http.Handler {
	return handleAlerts(a)
}

// ExportedHandleHosts exposes handleHosts for black-box tests.
func ExportedHandleHosts(tracker *storage.HostTracker) http.Handler {
	return handleHosts(tracker)
}

// ExportedHandlePrometheus exposes handlePrometheus for black-box tests.
func ExportedHandlePrometheus(repo storage.MetricRepository) http.Handler {
	return handlePrometheus(repo)
}
