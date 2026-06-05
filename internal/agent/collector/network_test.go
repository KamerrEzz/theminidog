package collector

import (
	"context"
	"errors"
	"testing"

	gopsutilnet "github.com/shirou/gopsutil/v4/net"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func makeIOCounters(name string, bytesRecv, bytesSent uint64) gopsutilnet.IOCountersStat {
	return gopsutilnet.IOCountersStat{
		Name:      name,
		BytesRecv: bytesRecv,
		BytesSent: bytesSent,
	}
}

// makeNetCollector builds a NetworkCollector with an injected ioFn and
// optional explicit ifaces allow-list.
func makeNetCollector(
	ioFn func(context.Context, bool) ([]gopsutilnet.IOCountersStat, error),
	ifaces []string,
) *NetworkCollector {
	return &NetworkCollector{
		host:   "test-host",
		ifaces: ifaces,
		ioFn:   ioFn,
		prev:   nil,
	}
}

// ── NetworkCollector.Name ─────────────────────────────────────────────────────

func TestNetworkCollector_Name(t *testing.T) {
	c := NewNetworkCollector("host1", nil)
	if c.Name() != "network" {
		t.Errorf("expected Name()=network, got %q", c.Name())
	}
}

// ── NetworkCollector.Collect — first call seeds and returns empty ─────────────

func TestNetworkCollector_Collect_FirstCall_ReturnsEmpty(t *testing.T) {
	ioFn := func(_ context.Context, _ bool) ([]gopsutilnet.IOCountersStat, error) {
		return []gopsutilnet.IOCountersStat{
			makeIOCounters("eth0", 1000, 500),
		}, nil
	}
	c := makeNetCollector(ioFn, nil)

	metrics, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("expected nil error on first call, got: %v", err)
	}
	if len(metrics) != 0 {
		t.Errorf("first call must return empty slice (seed), got %d metrics", len(metrics))
	}
	// prev must have been seeded
	if c.prev == nil {
		t.Error("prev state must be seeded after first call")
	}
}

// ── NetworkCollector.Collect — second call computes delta ────────────────────

func TestNetworkCollector_Collect_SecondCall_ComputesDelta(t *testing.T) {
	call := 0
	ioFn := func(_ context.Context, _ bool) ([]gopsutilnet.IOCountersStat, error) {
		call++
		if call == 1 {
			return []gopsutilnet.IOCountersStat{
				makeIOCounters("eth0", 1000, 500),
			}, nil
		}
		return []gopsutilnet.IOCountersStat{
			makeIOCounters("eth0", 2000, 1000),
		}, nil
	}
	c := makeNetCollector(ioFn, nil)

	// Seed
	_, _ = c.Collect(context.Background())

	// Delta call
	metrics, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics (bytes_in + bytes_out) for eth0, got %d", len(metrics))
	}

	byName := map[string]float64{}
	for _, m := range metrics {
		byName[m.Name] = m.Value
	}

	if v, ok := byName["net.bytes_in"]; !ok || v != 1000 {
		t.Errorf("expected net.bytes_in=1000, got %v (present=%v)", byName["net.bytes_in"], ok)
	}
	if v, ok := byName["net.bytes_out"]; !ok || v != 500 {
		t.Errorf("expected net.bytes_out=500, got %v (present=%v)", byName["net.bytes_out"], ok)
	}
}

// ── NetworkCollector.Collect — loopback "lo" always excluded ──────────────────

func TestNetworkCollector_Collect_LoopbackExcluded(t *testing.T) {
	call := 0
	ioFn := func(_ context.Context, _ bool) ([]gopsutilnet.IOCountersStat, error) {
		call++
		if call == 1 {
			return []gopsutilnet.IOCountersStat{
				makeIOCounters("lo", 5000, 5000),
				makeIOCounters("eth0", 100, 50),
			}, nil
		}
		return []gopsutilnet.IOCountersStat{
			makeIOCounters("lo", 6000, 6000),
			makeIOCounters("eth0", 200, 100),
		}, nil
	}
	c := makeNetCollector(ioFn, nil)
	_, _ = c.Collect(context.Background()) // seed

	metrics, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, m := range metrics {
		if m.Labels["iface"] == "lo" {
			t.Errorf("loopback 'lo' must never appear in output, found: %v", m)
		}
	}
	// eth0 should produce 2 metrics
	if len(metrics) != 2 {
		t.Errorf("expected 2 metrics for eth0, got %d", len(metrics))
	}
}

// ── NetworkCollector.Collect — negative delta clamped to 0 ───────────────────

func TestNetworkCollector_Collect_NegativeDelta_ClampedToZero(t *testing.T) {
	call := 0
	ioFn := func(_ context.Context, _ bool) ([]gopsutilnet.IOCountersStat, error) {
		call++
		if call == 1 {
			return []gopsutilnet.IOCountersStat{
				makeIOCounters("eth0", 5000, 3000),
			}, nil
		}
		// Counter wrapped/reset — lower value than before
		return []gopsutilnet.IOCountersStat{
			makeIOCounters("eth0", 100, 50),
		}, nil
	}
	c := makeNetCollector(ioFn, nil)
	_, _ = c.Collect(context.Background()) // seed

	metrics, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, m := range metrics {
		if m.Value < 0 {
			t.Errorf("metric %q: negative delta must be clamped to 0, got %v", m.Name, m.Value)
		}
		if m.Value != 0 {
			t.Errorf("metric %q: expected 0 (clamped), got %v", m.Name, m.Value)
		}
	}
}

// ── NetworkCollector.Collect — ioFn error ────────────────────────────────────

func TestNetworkCollector_Collect_IoFnError_ReturnsNilAndError(t *testing.T) {
	want := errors.New("network io failed")
	ioFn := func(_ context.Context, _ bool) ([]gopsutilnet.IOCountersStat, error) {
		return nil, want
	}
	c := makeNetCollector(ioFn, nil)

	metrics, err := c.Collect(context.Background())
	if metrics != nil {
		t.Errorf("expected nil metrics on error, got %v", metrics)
	}
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, want) {
		t.Errorf("expected wrapped error %v, got %v", want, err)
	}
}

// ── NetworkCollector.Collect — ifaces allow-list ─────────────────────────────

func TestNetworkCollector_Collect_IfacesAllowList(t *testing.T) {
	call := 0
	ioFn := func(_ context.Context, _ bool) ([]gopsutilnet.IOCountersStat, error) {
		call++
		if call == 1 {
			return []gopsutilnet.IOCountersStat{
				makeIOCounters("eth0", 100, 50),
				makeIOCounters("wlan0", 200, 100),
			}, nil
		}
		return []gopsutilnet.IOCountersStat{
			makeIOCounters("eth0", 200, 100),
			makeIOCounters("wlan0", 400, 200),
		}, nil
	}
	// Only allow eth0
	c := makeNetCollector(ioFn, []string{"eth0"})
	_, _ = c.Collect(context.Background()) // seed

	metrics, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, m := range metrics {
		if m.Labels["iface"] == "wlan0" {
			t.Errorf("wlan0 must be excluded by ifaces allow-list, found: %v", m)
		}
	}
	if len(metrics) != 2 {
		t.Errorf("expected 2 metrics for eth0 only, got %d", len(metrics))
	}
}

// ── NetworkCollector.Collect — label key is exactly "iface" ──────────────────

func TestNetworkCollector_Collect_LabelKeyIsIfaceExact(t *testing.T) {
	call := 0
	ioFn := func(_ context.Context, _ bool) ([]gopsutilnet.IOCountersStat, error) {
		call++
		return []gopsutilnet.IOCountersStat{makeIOCounters("eth0", uint64(call*100), uint64(call*50))}, nil
	}
	c := makeNetCollector(ioFn, nil)
	_, _ = c.Collect(context.Background()) // seed

	metrics, _ := c.Collect(context.Background())
	for i, m := range metrics {
		if _, ok := m.Labels["iface"]; !ok {
			t.Errorf("metric[%d]: missing label key 'iface'", i)
		}
		if len(m.Labels) != 1 {
			t.Errorf("metric[%d]: expected exactly 1 label, got %d: %v", i, len(m.Labels), m.Labels)
		}
	}
}

// ── NetworkCollector.Collect — metric names are exact ────────────────────────

func TestNetworkCollector_Collect_ExactMetricNames(t *testing.T) {
	call := 0
	ioFn := func(_ context.Context, _ bool) ([]gopsutilnet.IOCountersStat, error) {
		call++
		return []gopsutilnet.IOCountersStat{makeIOCounters("eth0", uint64(call*100), uint64(call*50))}, nil
	}
	c := makeNetCollector(ioFn, nil)
	_, _ = c.Collect(context.Background()) // seed

	metrics, _ := c.Collect(context.Background())
	expected := map[string]bool{
		"net.bytes_in":  true,
		"net.bytes_out": true,
	}
	for _, m := range metrics {
		if !expected[m.Name] {
			t.Errorf("unexpected metric name: %q", m.Name)
		}
		delete(expected, m.Name)
	}
	if len(expected) != 0 {
		t.Errorf("missing metric names: %v", expected)
	}
}
