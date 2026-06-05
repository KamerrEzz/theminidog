package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kamerrezz/theminidog/internal/model"
	"github.com/kamerrezz/theminidog/internal/server/api"
	"github.com/kamerrezz/theminidog/internal/server/storage"
)

// fakeLogRepo is a test double for storage.LogRepository.
type fakeLogRepo struct {
	insertErr  error
	insertN    int
	queryRows  []storage.LogQueryResult
	nextCursor int64
	queryErr   error
	pingErr    error
}

func (f *fakeLogRepo) Insert(_ context.Context, batch model.LogBatch) (int, error) {
	if f.insertErr != nil {
		return 0, f.insertErr
	}
	n := f.insertN
	if n == 0 {
		n = len(batch.Entries)
	}
	return n, nil
}

func (f *fakeLogRepo) Query(_ context.Context, _ storage.LogQueryParams) ([]storage.LogQueryResult, int64, error) {
	return f.queryRows, f.nextCursor, f.queryErr
}

func (f *fakeLogRepo) Ping(_ context.Context) error { return f.pingErr }

// makeValidBatch builds a minimal valid LogBatch with n entries.
func makeValidBatch(n int) model.LogBatch {
	entries := make([]model.LogEntry, n)
	for i := range entries {
		entries[i] = model.LogEntry{
			Time:    time.Now().UTC(),
			Host:    "web-01",
			Path:    "/var/log/app.log",
			Level:   model.LevelInfo,
			Message: "test message",
		}
	}
	return model.LogBatch{Host: "web-01", Entries: entries}
}

func bodyJSON(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewBuffer(b)
}

// --- HandleIngestLogs tests ---

func TestHandleIngestLogs_ValidBatch_Returns202(t *testing.T) {
	batch := makeValidBatch(3)
	repo := &fakeLogRepo{}
	handler := api.HandleIngestLogs(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bodyJSON(t, batch))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	resp := rw.Result()
	assertStatus(t, resp, http.StatusAccepted)

	var body map[string]int
	mustDecode(t, resp.Body, &body)
	if body["ingested"] != 3 {
		t.Fatalf("expected ingested=3, got %d", body["ingested"])
	}
}

func TestHandleIngestLogs_MalformedJSON_Returns400(t *testing.T) {
	repo := &fakeLogRepo{}
	handler := api.HandleIngestLogs(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", strings.NewReader(`{not valid json`))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	assertStatus(t, rw.Result(), http.StatusBadRequest)
}

func TestHandleIngestLogs_EmptyEntries_Returns400(t *testing.T) {
	batch := model.LogBatch{Host: "web-01", Entries: []model.LogEntry{}}
	repo := &fakeLogRepo{}
	handler := api.HandleIngestLogs(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bodyJSON(t, batch))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	assertStatus(t, rw.Result(), http.StatusBadRequest)
}

func TestHandleIngestLogs_1001Entries_Returns400(t *testing.T) {
	batch := makeValidBatch(1001)
	repo := &fakeLogRepo{}
	handler := api.HandleIngestLogs(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bodyJSON(t, batch))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	assertStatus(t, rw.Result(), http.StatusBadRequest)
}

func TestHandleIngestLogs_RepoInsertError_Returns500(t *testing.T) {
	batch := makeValidBatch(1)
	repo := &fakeLogRepo{insertErr: errors.New("db failure")}
	handler := api.HandleIngestLogs(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bodyJSON(t, batch))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	assertStatus(t, rw.Result(), http.StatusInternalServerError)
}

func TestHandleIngestLogs_InvalidLevel_Returns400(t *testing.T) {
	batch := model.LogBatch{
		Host: "web-01",
		Entries: []model.LogEntry{
			{Time: time.Now().UTC(), Host: "web-01", Path: "/app.log", Level: "critical", Message: "bad level"},
		},
	}
	repo := &fakeLogRepo{}
	handler := api.HandleIngestLogs(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bodyJSON(t, batch))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	assertStatus(t, rw.Result(), http.StatusBadRequest)
}

// --- HandleQueryLogs tests ---

func TestHandleQueryLogs_ValidParams_Returns200WithShape(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	rows := []storage.LogQueryResult{
		{ID: 2, Time: now, Host: "web-01", Path: "/app.log", Level: "error", Message: "disk full"},
		{ID: 1, Time: now.Add(-time.Second), Host: "web-01", Path: "/app.log", Level: "info", Message: "started"},
	}
	repo := &fakeLogRepo{queryRows: rows, nextCursor: 0}
	handler := api.HandleQueryLogs(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/query?host=web-01&limit=10", nil)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	resp := rw.Result()
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Entries    []storage.LogQueryResult `json:"entries"`
		NextCursor *int64                   `json:"next_cursor"`
	}
	mustDecode(t, resp.Body, &body)
	if len(body.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(body.Entries))
	}
	if body.NextCursor != nil {
		t.Fatalf("expected next_cursor=null, got %v", *body.NextCursor)
	}
}

func TestHandleQueryLogs_NoData_Returns200EmptySlice(t *testing.T) {
	repo := &fakeLogRepo{queryRows: nil, nextCursor: 0}
	handler := api.HandleQueryLogs(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/query", nil)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	resp := rw.Result()
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Entries    []storage.LogQueryResult `json:"entries"`
		NextCursor *int64                   `json:"next_cursor"`
	}
	mustDecode(t, resp.Body, &body)
	if body.Entries == nil {
		t.Fatal("expected entries to be [] not null")
	}
	if len(body.Entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(body.Entries))
	}
	if body.NextCursor != nil {
		t.Fatalf("expected next_cursor=null, got %v", *body.NextCursor)
	}
}

func TestHandleQueryLogs_InvalidLevel_Returns400(t *testing.T) {
	repo := &fakeLogRepo{}
	handler := api.HandleQueryLogs(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/query?level=critical", nil)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	assertStatus(t, rw.Result(), http.StatusBadRequest)
}

func TestHandleQueryLogs_InvalidFrom_Returns400(t *testing.T) {
	repo := &fakeLogRepo{}
	handler := api.HandleQueryLogs(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/query?from=notadate", nil)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	assertStatus(t, rw.Result(), http.StatusBadRequest)
}

func TestHandleQueryLogs_InvalidTo_Returns400(t *testing.T) {
	repo := &fakeLogRepo{}
	handler := api.HandleQueryLogs(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/query?to=notadate", nil)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	assertStatus(t, rw.Result(), http.StatusBadRequest)
}

func TestHandleQueryLogs_InvalidLimit_Returns400(t *testing.T) {
	repo := &fakeLogRepo{}
	handler := api.HandleQueryLogs(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/query?limit=abc", nil)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	assertStatus(t, rw.Result(), http.StatusBadRequest)
}

func TestHandleQueryLogs_InvalidCursor_Returns400(t *testing.T) {
	repo := &fakeLogRepo{}
	handler := api.HandleQueryLogs(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/query?cursor=notanint", nil)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	assertStatus(t, rw.Result(), http.StatusBadRequest)
}

func TestHandleQueryLogs_Pagination_NextCursorNonNull(t *testing.T) {
	var cursor int64 = 42
	repo := &fakeLogRepo{
		queryRows:  []storage.LogQueryResult{{ID: 42, Time: time.Now().UTC(), Host: "h", Path: "/p", Level: "info", Message: "m"}},
		nextCursor: cursor,
	}
	handler := api.HandleQueryLogs(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/query?limit=1", nil)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	resp := rw.Result()
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Entries    []storage.LogQueryResult `json:"entries"`
		NextCursor *int64                   `json:"next_cursor"`
	}
	mustDecode(t, resp.Body, &body)
	if body.NextCursor == nil {
		t.Fatal("expected next_cursor to be non-null")
	}
	if *body.NextCursor != cursor {
		t.Fatalf("expected next_cursor=%d, got %d", cursor, *body.NextCursor)
	}
}

func TestHandleQueryLogs_Pagination_NextCursorNull_WhenZero(t *testing.T) {
	repo := &fakeLogRepo{
		queryRows:  []storage.LogQueryResult{{ID: 1, Time: time.Now().UTC(), Host: "h", Path: "/p", Level: "info", Message: "m"}},
		nextCursor: 0,
	}
	handler := api.HandleQueryLogs(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/query?limit=10", nil)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	resp := rw.Result()
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		NextCursor *int64 `json:"next_cursor"`
	}
	mustDecode(t, resp.Body, &body)
	if body.NextCursor != nil {
		t.Fatalf("expected next_cursor=null when nextCursor=0, got %d", *body.NextCursor)
	}
}

func TestHandleQueryLogs_RepoQueryError_Returns500(t *testing.T) {
	repo := &fakeLogRepo{queryErr: errors.New("db failure")}
	handler := api.HandleQueryLogs(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/logs/query", nil)
	rw := httptest.NewRecorder()

	handler.ServeHTTP(rw, req)

	assertStatus(t, rw.Result(), http.StatusInternalServerError)
}
