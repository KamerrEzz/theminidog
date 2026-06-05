package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kamerrezz/theminidog/internal/server/api"
	"github.com/kamerrezz/theminidog/internal/server/storage"
)

func TestHandleHosts_NilTracker(t *testing.T) {
	handler := api.ExportedHandleHosts(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	hosts, ok := resp["hosts"]
	if !ok {
		t.Fatal("response missing 'hosts' key")
	}
	// Must be [] not null
	slice, ok := hosts.([]any)
	if !ok {
		t.Fatalf("expected hosts to be an array, got %T", hosts)
	}
	if len(slice) != 0 {
		t.Fatalf("expected empty array, got %d items", len(slice))
	}
}

func TestHandleHosts_ThreeHosts(t *testing.T) {
	tracker := storage.NewHostTracker(20*time.Second, 50*time.Second, nil)
	now := time.Now()
	tracker.Heartbeat("alpha")
	tracker.Heartbeat("bravo")
	tracker.Heartbeat("charlie")
	_ = now // suppress unused warning

	handler := api.ExportedHandleHosts(tracker)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	hosts, ok := resp["hosts"].([]any)
	if !ok {
		t.Fatalf("expected hosts array, got %T", resp["hosts"])
	}
	if len(hosts) != 3 {
		t.Fatalf("expected 3 hosts, got %d", len(hosts))
	}
	// Each entry must have host, status, last_seen fields
	for i, h := range hosts {
		entry, ok := h.(map[string]any)
		if !ok {
			t.Fatalf("hosts[%d]: expected object, got %T", i, h)
		}
		if _, ok := entry["host"]; !ok {
			t.Fatalf("hosts[%d]: missing 'host' field", i)
		}
		if _, ok := entry["status"]; !ok {
			t.Fatalf("hosts[%d]: missing 'status' field", i)
		}
		if _, ok := entry["last_seen"]; !ok {
			t.Fatalf("hosts[%d]: missing 'last_seen' field", i)
		}
	}
}

func TestHandleHosts_ContentType(t *testing.T) {
	handler := api.ExportedHandleHosts(nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}
}
