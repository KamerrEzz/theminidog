package api_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kamerrezz/theminidog/internal/model"
	"github.com/kamerrezz/theminidog/internal/server/api"
	"github.com/kamerrezz/theminidog/internal/server/storage"
)

// promFakeRepo is a MetricRepository stub that returns configurable hosts and
// query points for Prometheus handler tests.
type promFakeRepo struct {
	hosts    []string
	hostsErr error
	queryPts []storage.QueryPoint
	queryErr error
}

func (f *promFakeRepo) Ping(_ context.Context) error { return nil }

func (f *promFakeRepo) Insert(_ context.Context, _ model.MetricBatch) (int, error) {
	return 0, nil
}

func (f *promFakeRepo) Hosts(_ context.Context, _ time.Duration) ([]string, error) {
	return f.hosts, f.hostsErr
}

func (f *promFakeRepo) Query(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
	return f.queryPts, f.queryErr
}

// --- tests ---

func TestHandlePrometheus_StatusOK(t *testing.T) {
	repo := &promFakeRepo{
		hosts: []string{"web-01"},
		queryPts: []storage.QueryPoint{
			{Time: time.Now().UTC(), Value: 42.5},
		},
	}
	handler := api.ExportedHandlePrometheus(repo)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertStatus(t, rr.Result(), http.StatusOK)
}

func TestHandlePrometheus_ContentType(t *testing.T) {
	repo := &promFakeRepo{hosts: []string{"web-01"}}
	handler := api.ExportedHandlePrometheus(repo)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected Content-Type to start with text/plain, got %q", ct)
	}
}

func TestHandlePrometheus_ContainsHelpAndType(t *testing.T) {
	repo := &promFakeRepo{
		hosts: []string{"web-01"},
		queryPts: []storage.QueryPoint{
			{Time: time.Now().UTC(), Value: 55.0},
		},
	}
	handler := api.ExportedHandlePrometheus(repo)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	text := string(body)

	if !strings.Contains(text, "# HELP ") {
		t.Errorf("expected # HELP lines in response, got:\n%s", text)
	}
	if !strings.Contains(text, "# TYPE ") {
		t.Errorf("expected # TYPE lines in response, got:\n%s", text)
	}
}

func TestHandlePrometheus_MetricNamesUseUnderscores(t *testing.T) {
	repo := &promFakeRepo{
		hosts: []string{"web-01"},
		queryPts: []storage.QueryPoint{
			{Time: time.Now().UTC(), Value: 42.0},
		},
	}
	handler := api.ExportedHandlePrometheus(repo)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	text := string(body)

	// Must contain Prometheus-style name with underscores as the metric identifier.
	if !strings.Contains(text, "miniobserv_cpu_usage_pct") {
		t.Errorf("expected miniobserv_cpu_usage_pct in output, got:\n%s", text)
	}
	// The metric data lines must NOT start with a dotted name.
	// (# HELP / # TYPE comments may reference the original name — that is valid.)
	for _, line := range strings.Split(text, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "cpu.") || strings.HasPrefix(line, "mem.") ||
			strings.HasPrefix(line, "disk.") || strings.HasPrefix(line, "net.") {
			t.Errorf("data line must not start with a dot-separated name: %q", line)
		}
	}
}

func TestHandlePrometheus_HostLabel(t *testing.T) {
	repo := &promFakeRepo{
		hosts: []string{"web-01"},
		queryPts: []storage.QueryPoint{
			{Time: time.Now().UTC(), Value: 77.3},
		},
	}
	handler := api.ExportedHandlePrometheus(repo)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	text := string(body)

	if !strings.Contains(text, `host="web-01"`) {
		t.Errorf(`expected host="web-01" label in output, got:\n%s`, text)
	}
}

func TestHandlePrometheus_EmptyWhenNoHosts(t *testing.T) {
	// No active hosts — response should still be 200 with correct Content-Type but empty body.
	repo := &promFakeRepo{hosts: nil}
	handler := api.ExportedHandlePrometheus(repo)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assertStatus(t, rr.Result(), http.StatusOK)

	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected text/plain Content-Type even for empty response, got %q", ct)
	}
}
