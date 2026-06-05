package storage

import (
	"context"
	"sort"
	"sync"
	"time"
)

// HostStatus is a point-in-time liveness snapshot for one host.
type HostStatus struct {
	Host     string    `json:"host"`
	Status   string    `json:"status"`    // "ok" | "stale" | "down"
	LastSeen time.Time `json:"last_seen"`
}

// HostTracker tracks per-host heartbeats in memory.
// All exported methods are safe on a nil *HostTracker.
type HostTracker struct {
	mu         sync.RWMutex
	hosts      map[string]time.Time
	down       map[string]bool  // tracks hosts already reported down (dedupe onDown)
	staleAfter time.Duration
	downAfter  time.Duration
	onDown     func(host string) // nil-safe; fired once per ok/stale→down transition
	nowFn      func() time.Time  // injectable for tests; defaults to time.Now
}

// NewHostTracker constructs a HostTracker with the given thresholds and callback.
// onDown may be nil — it is called at most once per down transition per host.
func NewHostTracker(staleAfter, downAfter time.Duration, onDown func(host string)) *HostTracker {
	return &HostTracker{
		hosts:      make(map[string]time.Time),
		down:       make(map[string]bool),
		staleAfter: staleAfter,
		downAfter:  downAfter,
		onDown:     onDown,
		nowFn:      time.Now,
	}
}

// now returns the current time, using nowFn if set.
func (t *HostTracker) now() time.Time {
	if t.nowFn != nil {
		return t.nowFn()
	}
	return time.Now()
}

// Heartbeat records that host was seen now. Nil-safe.
// Also clears the down-tracking state so onDown fires again if the host goes down again.
func (t *HostTracker) Heartbeat(host string) {
	if t == nil || host == "" {
		return
	}
	t.mu.Lock()
	t.hosts[host] = t.now()
	delete(t.down, host) // recovered — onDown will fire again if it goes down again
	t.mu.Unlock()
}

// classify returns "ok", "stale", or "down" based on age relative to thresholds.
func (t *HostTracker) classify(lastSeen time.Time) string {
	age := t.now().Sub(lastSeen)
	switch {
	case age >= t.downAfter:
		return "down"
	case age >= t.staleAfter:
		return "stale"
	default:
		return "ok"
	}
}

// Status returns the current status for one host. Nil-safe (nil tracker returns "down").
// Unknown hosts are also returned as "down".
func (t *HostTracker) Status(host string) HostStatus {
	if t == nil {
		return HostStatus{Host: host, Status: "down"}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	ls, ok := t.hosts[host]
	if !ok {
		return HostStatus{Host: host, Status: "down"}
	}
	return HostStatus{Host: host, Status: t.classify(ls), LastSeen: ls}
}

// All returns a status snapshot for every known host, sorted by host name. Nil-safe.
// Returns an empty non-nil slice when the tracker is nil or no hosts are known.
func (t *HostTracker) All() []HostStatus {
	if t == nil {
		return []HostStatus{}
	}
	t.mu.RLock()
	out := make([]HostStatus, 0, len(t.hosts))
	for h, ls := range t.hosts {
		out = append(out, HostStatus{Host: h, Status: t.classify(ls), LastSeen: ls})
	}
	t.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Host < out[j].Host })
	return out
}

// Start launches a 10-second ticker that calls scan() to detect newly-down hosts.
// Blocks until ctx is cancelled; run as a goroutine. Nil-safe (returns immediately).
func (t *HostTracker) Start(ctx context.Context) {
	if t == nil {
		return
	}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.scan()
		}
	}
}

// scan classifies all hosts and fires onDown for hosts newly transitioning to down.
// Each host only fires onDown once per down transition (deduplicated via t.down map).
func (t *HostTracker) scan() {
	now := t.now()
	var newlyDown []string
	t.mu.Lock()
	for h, ls := range t.hosts {
		age := now.Sub(ls)
		isDown := age >= t.downAfter
		if isDown && !t.down[h] {
			t.down[h] = true
			newlyDown = append(newlyDown, h)
		}
	}
	t.mu.Unlock()
	if t.onDown == nil {
		return
	}
	for _, h := range newlyDown {
		t.onDown(h)
	}
}
