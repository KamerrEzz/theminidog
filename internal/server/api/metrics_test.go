package api_test

import (
	"bytes"
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

// --- helpers ---

func encodeJSON(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
	return &buf
}

func makeMetric(host, name string) model.Metric {
	return model.Metric{
		Time:  time.Now().UTC(),
		Host:  host,
		Name:  name,
		Value: 42.0,
	}
}

func makeBatch(host string, count int) model.MetricBatch {
	metrics := make([]model.Metric, count)
	for i := range metrics {
		metrics[i] = makeMetric(host, "cpu.usage_pct")
	}
	return model.MetricBatch{Host: host, Metrics: metrics}
}

// --- Ingest handler tests ---

func TestHandleIngest_ValidBatch(t *testing.T) {
	repo := &fakeRepo{insertN: 3}
	handler := api.HandleIngest(repo, nil)

	batch := makeBatch("web-01", 3)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics", encodeJSON(t, batch))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
	var resp map[string]int
	mustDecode(t, rr.Body, &resp)
	if resp["ingested"] != 3 {
		t.Fatalf("expected ingested=3, got %d", resp["ingested"])
	}
}

func TestHandleIngest_EmptyMetrics(t *testing.T) {
	repo := newFakeRepo()
	handler := api.HandleIngest(repo, nil)

	batch := model.MetricBatch{Host: "web-01", Metrics: []model.Metric{}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics", encodeJSON(t, batch))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleIngest_EmptyHost(t *testing.T) {
	repo := newFakeRepo()
	handler := api.HandleIngest(repo, nil)

	batch := model.MetricBatch{Host: "", Metrics: []model.Metric{makeMetric("", "cpu.usage_pct")}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics", encodeJSON(t, batch))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleIngest_NaNValue(t *testing.T) {
	repo := newFakeRepo()
	handler := api.HandleIngest(repo, nil)

	// NaN cannot be encoded as JSON normally; use raw JSON instead
	rawBody := `{"host":"web-01","metrics":[{"time":"` + time.Now().UTC().Format(time.RFC3339) +
		`","host":"web-01","name":"cpu.usage_pct","value":` + "NaN" + `}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics", strings.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// JSON decoder will fail on NaN (not valid JSON), returns 400
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleIngest_BatchOver1000(t *testing.T) {
	repo := newFakeRepo()
	handler := api.HandleIngest(repo, nil)

	batch := makeBatch("web-01", 1001)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics", encodeJSON(t, batch))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	msg := errBody(t, rr.Body)
	if !strings.Contains(msg, "1000") {
		t.Fatalf("expected error mentioning 1000, got %q", msg)
	}
}

func TestHandleIngest_RepoError(t *testing.T) {
	repo := &fakeRepo{insertN: 0, insertErr: errors.New("db failure")}
	handler := api.HandleIngest(repo, nil)

	batch := makeBatch("web-01", 2)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics", encodeJSON(t, batch))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// --- Query handler tests ---

func queryURL(from, to, host, name, bucket, agg string) string {
	return "/api/v1/metrics/query?from=" + from + "&to=" + to +
		"&host=" + host + "&name=" + name +
		"&bucket=" + bucket + "&agg=" + agg
}

func TestHandleQuery_ValidParams(t *testing.T) {
	pts := []storage.QueryPoint{
		{Time: nowUTC(), Value: 55.0},
		{Time: nowUTC().Add(-time.Minute), Value: 60.0},
	}
	repo := &fakeRepo{queryPts: pts}
	handler := api.HandleQuery(repo)

	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	url := queryURL(from, to, "web-01", "cpu.usage_pct", "5m", "avg")
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	mustDecode(t, rr.Body, &resp)
	if resp["host"] != "web-01" {
		t.Fatalf("expected host=web-01, got %v", resp["host"])
	}
	points, ok := resp["points"].([]any)
	if !ok || len(points) != 2 {
		t.Fatalf("expected 2 points, got %v", resp["points"])
	}
}

func TestHandleQuery_NoData(t *testing.T) {
	repo := &fakeRepo{queryPts: nil}
	handler := api.HandleQuery(repo)

	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	url := queryURL(from, to, "web-01", "cpu.usage_pct", "1m", "avg")
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	mustDecode(t, rr.Body, &resp)
	points, ok := resp["points"].([]any)
	if !ok {
		t.Fatalf("expected points array, got %T: %v", resp["points"], resp["points"])
	}
	if len(points) != 0 {
		t.Fatalf("expected empty points array, got %d items", len(points))
	}
}

func TestHandleQuery_InvalidBucket(t *testing.T) {
	repo := newFakeRepo()
	handler := api.HandleQuery(repo)

	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	url := queryURL(from, to, "web-01", "cpu.usage_pct", "2m", "avg")
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleQuery_FromAfterTo(t *testing.T) {
	repo := newFakeRepo()
	handler := api.HandleQuery(repo)

	from := time.Now().UTC().Format(time.RFC3339)
	to := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	url := queryURL(from, to, "web-01", "cpu.usage_pct", "1m", "avg")
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleQuery_RangeOver30Days(t *testing.T) {
	repo := newFakeRepo()
	handler := api.HandleQuery(repo)

	from := time.Now().UTC().Add(-31 * 24 * time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	url := queryURL(from, to, "web-01", "cpu.usage_pct", "1d", "avg")
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleQuery_NonCanonicalName(t *testing.T) {
	repo := newFakeRepo()
	handler := api.HandleQuery(repo)

	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	url := queryURL(from, to, "web-01", "my_custom_metric", "1m", "avg")
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleQuery_InvalidAgg(t *testing.T) {
	repo := newFakeRepo()
	handler := api.HandleQuery(repo)

	from := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Format(time.RFC3339)
	url := queryURL(from, to, "web-01", "cpu.usage_pct", "1m", "sum")
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleQuery_MissingFrom(t *testing.T) {
	repo := newFakeRepo()
	handler := api.HandleQuery(repo)

	to := time.Now().UTC().Format(time.RFC3339)
	url := "/api/v1/metrics/query?to=" + to + "&host=web-01&name=cpu.usage_pct&bucket=1m&agg=avg"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}
