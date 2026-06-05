package api

import (
	"encoding/json"
	"net/http"

	"github.com/kamerrezz/theminidog/internal/server/storage"
)

// handleHosts serves GET /api/v1/hosts (PUBLIC — no JWT).
// Returns {"hosts":[{host,status,last_seen}...]}. Nil-safe tracker returns [].
func handleHosts(tracker *storage.HostTracker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hosts := tracker.All() // nil-safe → []
		if hosts == nil {
			hosts = []storage.HostStatus{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"hosts": hosts}) //nolint:errcheck
	}
}
