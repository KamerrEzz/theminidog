package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kamerrezz/theminidog/internal/server/api"
)

func TestHandleHealthz(t *testing.T) {
	handler := api.HandleHealthz()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := strings.TrimSpace(rr.Body.String())
	if body != "ok" {
		t.Fatalf("expected body 'ok', got %q", body)
	}
}

func TestHandleReadyz_DBUp(t *testing.T) {
	repo := newFakeRepo() // pingErr = nil
	handler := api.HandleReadyz(repo)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := strings.TrimSpace(rr.Body.String())
	if body != "ok" {
		t.Fatalf("expected body 'ok', got %q", body)
	}
}

func TestHandleReadyz_DBDown(t *testing.T) {
	repo := &fakeRepo{pingErr: makePingErr()}
	handler := api.HandleReadyz(repo)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
	msg := errBody(t, rr.Body)
	if msg == "" {
		t.Fatal("expected non-empty error message in JSON body")
	}
}
