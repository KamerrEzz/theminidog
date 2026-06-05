package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/kamerrezz/theminidog/internal/server/storage"
)

func TestHostTracker_FreshHost_IsOk(t *testing.T) {
	tracker := storage.NewHostTracker(20*time.Second, 50*time.Second, nil)
	now := time.Now()
	storage.SetNowFn(tracker, func() time.Time { return now })

	tracker.Heartbeat("host-a")
	s := tracker.Status("host-a")
	if s.Status != "ok" {
		t.Fatalf("expected ok, got %q", s.Status)
	}
}

func TestHostTracker_StaleHost(t *testing.T) {
	staleAfter := 20 * time.Second
	downAfter := 50 * time.Second
	tracker := storage.NewHostTracker(staleAfter, downAfter, nil)

	base := time.Now()
	storage.SetNowFn(tracker, func() time.Time { return base })
	tracker.Heartbeat("host-b")

	// Query at base + staleAfter + 1s (stale, not yet down)
	storage.SetNowFn(tracker, func() time.Time { return base.Add(staleAfter + time.Second) })
	s := tracker.Status("host-b")
	if s.Status != "stale" {
		t.Fatalf("expected stale, got %q", s.Status)
	}
}

func TestHostTracker_DownHost(t *testing.T) {
	staleAfter := 20 * time.Second
	downAfter := 50 * time.Second
	tracker := storage.NewHostTracker(staleAfter, downAfter, nil)

	base := time.Now()
	storage.SetNowFn(tracker, func() time.Time { return base })
	tracker.Heartbeat("host-c")

	// Query at base + downAfter + 1s
	storage.SetNowFn(tracker, func() time.Time { return base.Add(downAfter + time.Second) })
	s := tracker.Status("host-c")
	if s.Status != "down" {
		t.Fatalf("expected down, got %q", s.Status)
	}
}

func TestHostTracker_HeartbeatResetsDown(t *testing.T) {
	staleAfter := 20 * time.Second
	downAfter := 50 * time.Second
	tracker := storage.NewHostTracker(staleAfter, downAfter, nil)

	base := time.Now()
	storage.SetNowFn(tracker, func() time.Time { return base })
	tracker.Heartbeat("host-d")

	// Advance past down threshold
	storage.SetNowFn(tracker, func() time.Time { return base.Add(downAfter + time.Second) })
	s := tracker.Status("host-d")
	if s.Status != "down" {
		t.Fatalf("expected down before reset, got %q", s.Status)
	}

	// Heartbeat resets the host — nowFn still returns base+downAfter+1s so status is ok (age=0)
	tracker.Heartbeat("host-d")
	s = tracker.Status("host-d")
	if s.Status != "ok" {
		t.Fatalf("expected ok after heartbeat reset, got %q", s.Status)
	}
}

func TestHostTracker_All_Sorted(t *testing.T) {
	tracker := storage.NewHostTracker(20*time.Second, 50*time.Second, nil)
	now := time.Now()
	storage.SetNowFn(tracker, func() time.Time { return now })

	tracker.Heartbeat("charlie")
	tracker.Heartbeat("alpha")
	tracker.Heartbeat("bravo")

	all := tracker.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 hosts, got %d", len(all))
	}
	expected := []string{"alpha", "bravo", "charlie"}
	for i, s := range all {
		if s.Host != expected[i] {
			t.Fatalf("index %d: expected %q, got %q", i, expected[i], s.Host)
		}
	}
}

func TestHostTracker_OnDown_FiredOnce(t *testing.T) {
	staleAfter := 20 * time.Second
	downAfter := 50 * time.Second

	callCount := 0
	onDown := func(_ string) { callCount++ }

	tracker := storage.NewHostTracker(staleAfter, downAfter, onDown)
	base := time.Now()
	storage.SetNowFn(tracker, func() time.Time { return base })
	tracker.Heartbeat("host-e")

	// Advance past down threshold
	storage.SetNowFn(tracker, func() time.Time { return base.Add(downAfter + time.Second) })

	// First scan: should fire onDown once
	storage.ScanOnce(tracker)
	if callCount != 1 {
		t.Fatalf("expected onDown called once after first scan, got %d", callCount)
	}

	// Second scan: should NOT fire again (still down, already tracked)
	storage.ScanOnce(tracker)
	if callCount != 1 {
		t.Fatalf("expected onDown still called once after second scan, got %d", callCount)
	}
}

func TestHostTracker_NilSafe(t *testing.T) {
	var tracker *storage.HostTracker

	// None of these should panic
	tracker.Heartbeat("host-f")

	s := tracker.Status("host-f")
	if s.Status != "down" {
		t.Fatalf("nil tracker Status should return down, got %q", s.Status)
	}

	all := tracker.All()
	if all == nil {
		t.Fatal("nil tracker All() should return non-nil empty slice")
	}
	if len(all) != 0 {
		t.Fatalf("nil tracker All() should return empty slice, got %d", len(all))
	}

	// Start should return immediately on a cancelled ctx
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	done := make(chan struct{})
	go func() {
		tracker.Start(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("nil tracker Start did not return quickly")
	}
}

func TestHostTracker_NilOnDown(t *testing.T) {
	staleAfter := 20 * time.Second
	downAfter := 50 * time.Second

	// nil onDown — scan must not panic
	tracker := storage.NewHostTracker(staleAfter, downAfter, nil)
	base := time.Now()
	storage.SetNowFn(tracker, func() time.Time { return base })
	tracker.Heartbeat("host-g")

	storage.SetNowFn(tracker, func() time.Time { return base.Add(downAfter + time.Second) })
	// Should not panic
	storage.ScanOnce(tracker)
}
