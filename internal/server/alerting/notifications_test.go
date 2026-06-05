package alerting_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/kamerrezz/theminidog/internal/server/alerting"
)

// ---- ParseNotifications tests ----

func TestParseNotifications_Empty(t *testing.T) {
	notifiers, err := alerting.ParseNotifications("")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if notifiers != nil {
		t.Fatalf("expected nil slice, got %v", notifiers)
	}
}

func TestParseNotifications_Whitespace(t *testing.T) {
	notifiers, err := alerting.ParseNotifications("   ")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if notifiers != nil {
		t.Fatalf("expected nil slice, got %v", notifiers)
	}
}

func TestParseNotifications_SingleURL(t *testing.T) {
	raw := `[{"type":"webhook","url":"http://example.com/hook"}]`
	notifiers, err := alerting.ParseNotifications(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifiers) != 1 {
		t.Fatalf("expected 1 notifier, got %d", len(notifiers))
	}
}

func TestParseNotifications_InvalidJSON(t *testing.T) {
	_, err := alerting.ParseNotifications("{bad json")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseNotifications_UnsupportedType(t *testing.T) {
	raw := `[{"type":"email","url":"mailto:x@example.com"}]`
	_, err := alerting.ParseNotifications(raw)
	if err == nil {
		t.Fatal("expected error for unsupported type 'email', got nil")
	}
}

// ---- WebhookNotifier tests (via export shim or direct construction) ----

func TestWebhookNotifier_PostsJSON(t *testing.T) {
	var (
		mu          sync.Mutex
		gotMethod   string
		gotCT       string
		gotBodyKeys map[string]any
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBodyKeys); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := alerting.NewWebhookNotifier(srv.URL)
	ev := alerting.NotificationEvent{
		Event:   "firing",
		Rule:    alerting.Rule{Host: "web-01", Name: "cpu.usage_pct", Op: alerting.OpGT, Threshold: 90.0, For: 5 * time.Minute},
		Value:   95.0,
		FiredAt: time.Now().UTC(),
	}
	if err := n.Notify(context.Background(), ev); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotCT != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", gotCT)
	}
	if _, ok := gotBodyKeys["event"]; !ok {
		t.Errorf("expected body to have 'event' key, got %v", gotBodyKeys)
	}
}

func TestWebhookNotifier_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := alerting.NewWebhookNotifier(srv.URL)
	ev := alerting.NotificationEvent{
		Event:   "firing",
		Rule:    alerting.Rule{Host: "web-01", Name: "cpu.usage_pct", Op: alerting.OpGT, Threshold: 90.0, For: 5 * time.Minute},
		Value:   95.0,
		FiredAt: time.Now().UTC(),
	}
	err := n.Notify(context.Background(), ev)
	if err == nil {
		t.Fatal("expected non-nil error for 500 response, got nil")
	}
}
