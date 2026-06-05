package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/kamerrezz/theminidog/internal/model"
	"github.com/kamerrezz/theminidog/internal/server/storage"
)

// HandleIngestLogs returns a handler for POST /api/v1/logs.
// It decodes, validates, and persists a LogBatch to storage.
func HandleIngestLogs(repo storage.LogRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var batch model.LogBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		if err := batch.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
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

// HandleQueryLogs returns a handler for GET /api/v1/logs/query.
// It parses optional filter params, validates them, and returns paginated log results.
func HandleQueryLogs(repo storage.LogRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		params := storage.LogQueryParams{
			Host:  q.Get("host"),
			Level: q.Get("level"),
			Q:     q.Get("q"),
		}

		if fromStr := q.Get("from"); fromStr != "" {
			t, err := time.Parse(time.RFC3339, fromStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid 'from': must be RFC3339")
				return
			}
			params.From = t
		}
		if toStr := q.Get("to"); toStr != "" {
			t, err := time.Parse(time.RFC3339, toStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid 'to': must be RFC3339")
				return
			}
			params.To = t
		}
		if limitStr := q.Get("limit"); limitStr != "" {
			n, err := strconv.Atoi(limitStr)
			if err != nil || n < 0 {
				writeError(w, http.StatusBadRequest, "invalid 'limit': must be a non-negative integer")
				return
			}
			params.Limit = n
		}
		if cursorStr := q.Get("cursor"); cursorStr != "" {
			n, err := strconv.ParseInt(cursorStr, 10, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid 'cursor'")
				return
			}
			params.Cursor = n
		}

		if err := params.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		results, nextCursor, err := repo.Query(r.Context(), params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "query error")
			return
		}
		if results == nil {
			results = []storage.LogQueryResult{}
		}

		// next_cursor is null in JSON when 0 (no more pages).
		type response struct {
			Entries    []storage.LogQueryResult `json:"entries"`
			NextCursor *int64                   `json:"next_cursor"`
		}
		resp := response{Entries: results}
		if nextCursor > 0 {
			resp.NextCursor = &nextCursor
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
