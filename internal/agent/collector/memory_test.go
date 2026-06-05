package collector

import (
	"context"
	"errors"
	"testing"

	gopsutilmem "github.com/shirou/gopsutil/v4/mem"
)

// ── MemoryCollector.Name ──────────────────────────────────────────────────────

func TestMemoryCollector_Name(t *testing.T) {
	c := NewMemoryCollector("host1")
	if c.Name() != "memory" {
		t.Errorf("expected Name()=memory, got %q", c.Name())
	}
}

// ── MemoryCollector.Collect — happy path ──────────────────────────────────────

func TestMemoryCollector_Collect_ReturnsThreeMetrics(t *testing.T) {
	stub := &gopsutilmem.VirtualMemoryStat{
		Total:       8589934592,
		Used:        2147483648,
		UsedPercent: 25.0,
	}
	c := &MemoryCollector{
		host:   "test-host",
		statFn: func(_ context.Context) (*gopsutilmem.VirtualMemoryStat, error) { return stub, nil },
	}

	metrics, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(metrics) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(metrics))
	}

	// Collect expected names and verify all present
	names := map[string]float64{
		"mem.used_pct":    25.0,
		"mem.used_bytes":  float64(stub.Used),
		"mem.total_bytes": float64(stub.Total),
	}
	for _, m := range metrics {
		expected, ok := names[m.Name]
		if !ok {
			t.Errorf("unexpected metric name: %q", m.Name)
			continue
		}
		if m.Value != expected {
			t.Errorf("metric %q: expected value=%v, got %v", m.Name, expected, m.Value)
		}
		if m.Host != "test-host" {
			t.Errorf("metric %q: expected host=test-host, got %q", m.Name, m.Host)
		}
		if m.Time.IsZero() {
			t.Errorf("metric %q: Time must not be zero", m.Name)
		}
		delete(names, m.Name)
	}
	if len(names) != 0 {
		t.Errorf("missing metrics: %v", names)
	}
}

// ── MemoryCollector.Collect — empty labels ────────────────────────────────────

func TestMemoryCollector_Collect_AllLabelsNilOrEmpty(t *testing.T) {
	stub := &gopsutilmem.VirtualMemoryStat{Total: 1024, Used: 512, UsedPercent: 50.0}
	c := &MemoryCollector{
		host:   "h",
		statFn: func(_ context.Context) (*gopsutilmem.VirtualMemoryStat, error) { return stub, nil },
	}

	metrics, _ := c.Collect(context.Background())
	for i, m := range metrics {
		if len(m.Labels) != 0 {
			t.Errorf("metric[%d] %q: expected empty labels, got %v", i, m.Name, m.Labels)
		}
	}
}

// ── MemoryCollector.Collect — error path ─────────────────────────────────────

func TestMemoryCollector_Collect_StatFnError_ReturnsNilAndError(t *testing.T) {
	want := errors.New("mem stat failed")
	c := &MemoryCollector{
		host:   "h",
		statFn: func(_ context.Context) (*gopsutilmem.VirtualMemoryStat, error) { return nil, want },
	}

	metrics, err := c.Collect(context.Background())
	if metrics != nil {
		t.Errorf("expected nil metrics on error, got %v", metrics)
	}
	if err == nil {
		t.Fatal("expected non-nil error, got nil")
	}
	if !errors.Is(err, want) {
		t.Errorf("expected wrapped error %v, got %v", want, err)
	}
}

// ── MemoryCollector.Collect — exact metric names ──────────────────────────────

func TestMemoryCollector_Collect_ExactMetricNames(t *testing.T) {
	stub := &gopsutilmem.VirtualMemoryStat{Total: 100, Used: 50, UsedPercent: 50.0}
	c := &MemoryCollector{
		host:   "h",
		statFn: func(_ context.Context) (*gopsutilmem.VirtualMemoryStat, error) { return stub, nil },
	}

	metrics, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedNames := map[string]bool{
		"mem.used_pct":    true,
		"mem.used_bytes":  true,
		"mem.total_bytes": true,
	}
	for _, m := range metrics {
		if !expectedNames[m.Name] {
			t.Errorf("unexpected metric name: %q", m.Name)
		}
		delete(expectedNames, m.Name)
	}
	if len(expectedNames) != 0 {
		t.Errorf("missing metric names: %v", expectedNames)
	}
}
