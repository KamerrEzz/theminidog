package api

import (
	"net/http"

	"github.com/kamerrezz/theminidog/internal/server/alerting"
)

// ExportedHandleAlerts exposes the unexported handleAlerts function for
// black-box tests in the api_test package.
func ExportedHandleAlerts(a alerting.AlertReader) http.Handler {
	return handleAlerts(a)
}
