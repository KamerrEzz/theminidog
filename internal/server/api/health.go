package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/kamerrezz/theminidog/internal/server/storage"
)

// HandleHealthz returns a handler for the liveness probe.
// It always returns 200 "ok" with no database interaction.
func HandleHealthz() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}
}

// HandleReadyz returns a handler for the readiness probe.
// It calls repo.Ping to verify DB connectivity; returns 503 on failure.
func HandleReadyz(repo storage.MetricRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := repo.Ping(context.Background()); err != nil {
			writeError(w, http.StatusServiceUnavailable, "db unavailable")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}
}
