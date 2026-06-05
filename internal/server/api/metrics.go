package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/kamerrezz/theminidog/internal/model"
	"github.com/kamerrezz/theminidog/internal/server/storage"
)

const maxBatchSize = 1000

// HandleIngest returns a handler for POST /api/v1/metrics.
// It decodes, validates, and persists a MetricBatch to storage.
func HandleIngest(repo storage.MetricRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var batch model.MetricBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if len(batch.Metrics) == 0 {
			writeError(w, http.StatusBadRequest, "metrics must not be empty")
			return
		}
		if err := batch.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if len(batch.Metrics) > maxBatchSize {
			writeError(w, http.StatusBadRequest, "batch exceeds maximum size of 1000")
			return
		}
		n, err := repo.Insert(r.Context(), batch)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "storage error")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]int{"ingested": n})
	}
}

// HandleQuery returns a handler for GET /api/v1/metrics/query.
// It parses, validates query parameters, and returns time-bucketed metric points.
func HandleQuery(repo storage.MetricRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		fromStr := q.Get("from")
		toStr := q.Get("to")

		from, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid 'from': must be RFC3339")
			return
		}
		to, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid 'to': must be RFC3339")
			return
		}

		bucket := q.Get("bucket")
		if bucket == "" {
			bucket = "1m"
		}
		agg := q.Get("agg")
		if agg == "" {
			agg = "avg"
		}

		params := storage.QueryParams{
			Host:   q.Get("host"),
			Name:   q.Get("name"),
			From:   from,
			To:     to,
			Bucket: bucket,
			Agg:    agg,
		}
		if err := params.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		points, err := repo.Query(r.Context(), params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "query error")
			return
		}
		if points == nil {
			points = []storage.QueryPoint{} // return [] not null
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"host":   params.Host,
			"name":   params.Name,
			"bucket": params.Bucket,
			"agg":    params.Agg,
			"points": points,
		})
	}
}
