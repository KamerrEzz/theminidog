package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kamerrezz/theminidog/internal/model"
)

// noopSleep is a sleepFn that returns immediately without delay.
func noopSleep(_ context.Context, _ time.Duration) error { return nil }

// cancelledSleep is a sleepFn that always returns the context's error.
func cancelledSleep(ctx context.Context, _ time.Duration) error {
	return ctx.Err()
}

// constRand always returns 0.5 so jitter is exactly 0 (factor = 1.0).
func constRand() float64 { return 0.5 }

// makeBatch returns a minimal valid MetricBatch for use in tests.
func makeBatch() model.MetricBatch {
	return model.MetricBatch{
		Host: "test-host",
		Metrics: []model.Metric{
			{
				Time:  time.Now(),
				Host:  "test-host",
				Name:  "cpu.usage_pct",
				Value: 42.0,
				Labels: map[string]string{"core": "total"},
			},
		},
	}
}

// responseSequence builds an httptest.Server that returns status codes in
// order, then loops on the last one.
func responseSequence(t *testing.T, codes []int) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var count atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(count.Add(1)) - 1
		if n >= len(codes) {
			n = len(codes) - 1
		}
		w.WriteHeader(codes[n])
	}))
	t.Cleanup(srv.Close)
	return srv, &count
}

// TestSend_503x2_then_202 verifies the sender retries on 503 and succeeds on 202.
func TestSend_503x2_then_202(t *testing.T) {
	srv, count := responseSequence(t, []int{503, 503, 202})

	cfg := BackoffConfig{Base: time.Millisecond, Max: 10 * time.Millisecond, Jitter: 0}
	s := NewHTTPSender(srv.URL, cfg, nil).
		withRandFn(constRand).
		withSleepFn(noopSleep)

	err := s.Send(context.Background(), makeBatch())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := count.Load(); got != 3 {
		t.Fatalf("expected 3 requests, got %d", got)
	}
}

// TestSend_400_no_retry verifies a 400 response causes exactly 1 request and a permanent error.
func TestSend_400_no_retry(t *testing.T) {
	srv, count := responseSequence(t, []int{400})

	cfg := BackoffConfig{Base: time.Millisecond, Max: 10 * time.Millisecond, Jitter: 0}
	s := NewHTTPSender(srv.URL, cfg, nil).
		withRandFn(constRand).
		withSleepFn(noopSleep)

	err := s.Send(context.Background(), makeBatch())
	if err == nil {
		t.Fatal("expected error for 400, got nil")
	}
	if got := count.Load(); got != 1 {
		t.Fatalf("expected exactly 1 request, got %d", got)
	}
}

// TestSend_404_no_retry verifies other 4xx responses also cause no retry.
func TestSend_404_no_retry(t *testing.T) {
	srv, count := responseSequence(t, []int{404})

	cfg := BackoffConfig{Base: time.Millisecond, Max: 10 * time.Millisecond, Jitter: 0}
	s := NewHTTPSender(srv.URL, cfg, nil).
		withRandFn(constRand).
		withSleepFn(noopSleep)

	err := s.Send(context.Background(), makeBatch())
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if got := count.Load(); got != 1 {
		t.Fatalf("expected exactly 1 request for 4xx, got %d", got)
	}
}

// TestSend_202_success verifies a 202 returns nil immediately.
func TestSend_202_success(t *testing.T) {
	srv, count := responseSequence(t, []int{202})

	cfg := BackoffConfig{Base: time.Millisecond, Max: 10 * time.Millisecond, Jitter: 0}
	s := NewHTTPSender(srv.URL, cfg, nil).
		withRandFn(constRand).
		withSleepFn(noopSleep)

	err := s.Send(context.Background(), makeBatch())
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if got := count.Load(); got != 1 {
		t.Fatalf("expected exactly 1 request, got %d", got)
	}
}

// TestSend_ctx_cancel_during_sleep verifies that ctx cancellation during sleep
// stops the sender immediately without sending further requests.
func TestSend_ctx_cancel_during_sleep(t *testing.T) {
	var requestCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(503)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())

	// sleepFn that cancels ctx on first call (simulates cancel during backoff)
	called := false
	sleepFn := func(c context.Context, d time.Duration) error {
		if !called {
			called = true
			return nil // let the first request happen
		}
		// On second sleep (after first 503), cancel context and return ctx.Err()
		cancel()
		return c.Err()
	}

	cfg := BackoffConfig{Base: time.Millisecond, Max: 10 * time.Millisecond, Jitter: 0}
	s := NewHTTPSender(srv.URL, cfg, nil).
		withRandFn(constRand).
		withSleepFn(sleepFn)

	err := s.Send(ctx, makeBatch())
	if err == nil {
		t.Fatal("expected ctx error, got nil")
	}
	// Only the first attempt should have been made (before cancel during sleep)
	if got := requestCount.Load(); got != 1 {
		t.Fatalf("expected 1 request before cancel, got %d", got)
	}
}

// TestSend_content_type verifies every request carries Content-Type: application/json.
func TestSend_content_type(t *testing.T) {
	var badContentType bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			badContentType = true
		}
		w.WriteHeader(202)
	}))
	t.Cleanup(srv.Close)

	cfg := BackoffConfig{Base: time.Millisecond, Max: 10 * time.Millisecond, Jitter: 0}
	s := NewHTTPSender(srv.URL, cfg, nil).
		withRandFn(constRand).
		withSleepFn(noopSleep)

	if err := s.Send(context.Background(), makeBatch()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if badContentType {
		t.Fatal("Content-Type was not application/json")
	}
}

// TestSend_content_type_on_retry verifies Content-Type header is set on retry requests too.
func TestSend_content_type_on_retry(t *testing.T) {
	var badContentType bool
	var reqCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		if r.Header.Get("Content-Type") != "application/json" {
			badContentType = true
		}
		if reqCount < 3 {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(202)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := BackoffConfig{Base: time.Millisecond, Max: 10 * time.Millisecond, Jitter: 0}
	s := NewHTTPSender(srv.URL, cfg, nil).
		withRandFn(constRand).
		withSleepFn(noopSleep)

	if err := s.Send(context.Background(), makeBatch()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if badContentType {
		t.Fatal("Content-Type was not application/json on all retry requests")
	}
}

// TestSend_body_is_valid_json verifies the request body is valid JSON.
func TestSend_body_is_valid_json(t *testing.T) {
	batch := makeBatch()
	wantBody, _ := json.Marshal(batch)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		buf.ReadFrom(r.Body)
		if !bytes.Equal(buf.Bytes(), wantBody) {
			t.Errorf("body mismatch:\ngot  %s\nwant %s", buf.Bytes(), wantBody)
		}
		w.WriteHeader(202)
	}))
	t.Cleanup(srv.Close)

	cfg := BackoffConfig{Base: time.Millisecond, Max: 10 * time.Millisecond, Jitter: 0}
	s := NewHTTPSender(srv.URL, cfg, nil).
		withRandFn(constRand).
		withSleepFn(noopSleep)

	if err := s.Send(context.Background(), batch); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestWaitFor_table tests the waitFor function with deterministic inputs.
func TestWaitFor_table(t *testing.T) {
	cfg := DefaultBackoff()

	tests := []struct {
		attempt int
		wantMin time.Duration
		wantMax time.Duration
	}{
		// attempt=0 → always 0, no wait before first try
		{0, 0, 0},
		// attempt=1: base=1s, jitter=0.25 → range [0.75s, 1.25s]
		{1, 750 * time.Millisecond, 1250 * time.Millisecond},
		// attempt=2: base*2=2s, range [1.5s, 2.5s]
		{2, 1500 * time.Millisecond, 2500 * time.Millisecond},
		// attempt=7: base*2^6=64s > max(60s) → capped at 60s, range [45s, 75s]
		{7, 45 * time.Second, 75 * time.Second},
	}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			// Use constRand (returns 0.5) for deterministic mid-range test
			got := waitFor(tc.attempt, cfg, constRand)
			if tc.attempt == 0 {
				if got != 0 {
					t.Fatalf("attempt=0: expected 0, got %v", got)
				}
				return
			}
			if got < tc.wantMin || got > tc.wantMax {
				t.Fatalf("attempt=%d: got %v, want [%v, %v]",
					tc.attempt, got, tc.wantMin, tc.wantMax)
			}
		})
	}
}

// TestWaitFor_jitter verifies jitter pushes values into the full range.
func TestWaitFor_jitter(t *testing.T) {
	cfg := DefaultBackoff()

	// With rand=0.0: factor = 1 + 0.25*(0*2-1) = 0.75
	low := waitFor(1, cfg, func() float64 { return 0.0 })
	// With rand=1.0: factor = 1 + 0.25*(1*2-1) = 1.25
	high := waitFor(1, cfg, func() float64 { return 1.0 })

	if low >= high {
		t.Fatalf("expected low < high with full jitter range; got low=%v high=%v", low, high)
	}
	want_low := 750 * time.Millisecond
	want_high := 1250 * time.Millisecond
	if low < want_low || low > want_high {
		t.Fatalf("low jitter %v out of expected range [%v, %v]", low, want_low, want_high)
	}
	if high < want_low || high > want_high {
		t.Fatalf("high jitter %v out of expected range [%v, %v]", high, want_low, want_high)
	}
}

// TestWaitFor_cap verifies no result exceeds max*1.25 (jitter upper bound).
func TestWaitFor_cap(t *testing.T) {
	cfg := DefaultBackoff() // Max = 60s, Jitter = 0.25

	for attempt := 7; attempt <= 20; attempt++ {
		got := waitFor(attempt, cfg, func() float64 { return 1.0 }) // max jitter
		cap := time.Duration(float64(cfg.Max) * (1 + cfg.Jitter))
		if got > cap {
			t.Fatalf("attempt=%d: %v exceeds cap %v", attempt, got, cap)
		}
	}
}
