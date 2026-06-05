package collector

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ── CPU collector helpers ─────────────────────────────────────────────────────

// makeCPUStatFn returns a statFn stub that delegates based on the percpu flag.
func makeCPUStatFn(
	totalResult []float64, totalErr error,
	perCoreResult []float64, perCoreErr error,
) func(ctx context.Context, interval time.Duration, percpu bool) ([]float64, error) {
	return func(_ context.Context, _ time.Duration, percpu bool) ([]float64, error) {
		if percpu {
			return perCoreResult, perCoreErr
		}
		return totalResult, totalErr
	}
}

// ── CPUCollector.Name ─────────────────────────────────────────────────────────

func TestCPUCollector_Name(t *testing.T) {
	c := NewCPUCollector("host1")
	if c.Name() != "cpu" {
		t.Errorf("expected Name()=cpu, got %q", c.Name())
	}
}

// ── CPUCollector.Collect — happy path ─────────────────────────────────────────

func TestCPUCollector_Collect_ReturnsAggregateAndPerCore(t *testing.T) {
	// Stub: aggregate=40.0, per-core=[30.0, 50.0]
	statFn := makeCPUStatFn(
		[]float64{40.0}, nil,
		[]float64{30.0, 50.0}, nil,
	)
	c := &CPUCollector{host: "test-host", statFn: statFn}

	metrics, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(metrics) != 3 {
		t.Fatalf("expected 3 metrics (total + 2 cores), got %d", len(metrics))
	}

	// All must have canonical name
	for i, m := range metrics {
		if m.Name != "cpu.usage_pct" {
			t.Errorf("metric[%d]: expected name=cpu.usage_pct, got %q", i, m.Name)
		}
		if m.Host != "test-host" {
			t.Errorf("metric[%d]: expected host=test-host, got %q", i, m.Host)
		}
		if m.Time.IsZero() {
			t.Errorf("metric[%d]: Time must not be zero", i)
		}
	}

	// First metric must be total
	total := metrics[0]
	if total.Labels["core"] != "total" {
		t.Errorf("metrics[0]: expected label core=total, got %q", total.Labels["core"])
	}
	if total.Value != 40.0 {
		t.Errorf("metrics[0]: expected value=40.0, got %v", total.Value)
	}

	// Core 0
	core0 := metrics[1]
	if core0.Labels["core"] != "0" {
		t.Errorf("metrics[1]: expected label core=0, got %q", core0.Labels["core"])
	}
	if core0.Value != 30.0 {
		t.Errorf("metrics[1]: expected value=30.0, got %v", core0.Value)
	}

	// Core 1
	core1 := metrics[2]
	if core1.Labels["core"] != "1" {
		t.Errorf("metrics[2]: expected label core=1, got %q", core1.Labels["core"])
	}
	if core1.Value != 50.0 {
		t.Errorf("metrics[2]: expected value=50.0, got %v", core1.Value)
	}
}

// ── CPUCollector.Collect — error on aggregate ─────────────────────────────────

func TestCPUCollector_Collect_AggregateError_ReturnsNilAndError(t *testing.T) {
	want := errors.New("cpu aggregate failed")
	statFn := makeCPUStatFn(nil, want, nil, nil)
	c := &CPUCollector{host: "test-host", statFn: statFn}

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

// ── CPUCollector.Collect — empty aggregate result ─────────────────────────────

func TestCPUCollector_Collect_EmptyAggregateResult_ReturnsError(t *testing.T) {
	statFn := makeCPUStatFn(
		[]float64{}, nil, // empty total
		[]float64{30.0}, nil,
	)
	c := &CPUCollector{host: "test-host", statFn: statFn}

	metrics, err := c.Collect(context.Background())
	if metrics != nil {
		t.Errorf("expected nil metrics on empty result, got %v", metrics)
	}
	if err == nil {
		t.Fatal("expected non-nil error for empty aggregate result")
	}
	if !containsSubstr(err.Error(), "empty result") {
		t.Errorf("expected error to mention 'empty result', got: %v", err)
	}
}

// ── CPUCollector.Collect — error on per-core ─────────────────────────────────

func TestCPUCollector_Collect_PerCoreError_ReturnsNilAndError(t *testing.T) {
	want := errors.New("per-core failed")
	statFn := makeCPUStatFn(
		[]float64{40.0}, nil,
		nil, want,
	)
	c := &CPUCollector{host: "test-host", statFn: statFn}

	metrics, err := c.Collect(context.Background())
	if metrics != nil {
		t.Errorf("expected nil metrics on per-core error, got %v", metrics)
	}
	if err == nil {
		t.Fatal("expected non-nil error, got nil")
	}
}

// ── CPUCollector.Collect — label correctness ─────────────────────────────────

func TestCPUCollector_Collect_SingleCore_LabelsCorrect(t *testing.T) {
	statFn := makeCPUStatFn(
		[]float64{55.5}, nil,
		[]float64{55.5}, nil,
	)
	c := &CPUCollector{host: "h", statFn: statFn}

	metrics, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2 metrics: total + core 0
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(metrics))
	}

	if metrics[0].Labels["core"] != "total" {
		t.Errorf("expected core=total, got %q", metrics[0].Labels["core"])
	}
	if metrics[1].Labels["core"] != "0" {
		t.Errorf("expected core=0, got %q", metrics[1].Labels["core"])
	}
}

// ── CPUCollector.Collect — no extra label keys ────────────────────────────────

func TestCPUCollector_Collect_LabelsHaveOnlyCoreKey(t *testing.T) {
	statFn := makeCPUStatFn(
		[]float64{10.0}, nil,
		[]float64{10.0, 20.0}, nil,
	)
	c := &CPUCollector{host: "h", statFn: statFn}

	metrics, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, m := range metrics {
		if len(m.Labels) != 1 {
			t.Errorf("metric[%d]: expected exactly 1 label key, got %d: %v", i, len(m.Labels), m.Labels)
		}
		if _, ok := m.Labels["core"]; !ok {
			t.Errorf("metric[%d]: missing label key 'core'", i)
		}
	}
}

// ── No real OS calls ──────────────────────────────────────────────────────────

// This test verifies that injecting a statFn that panics on real OS calls
// is never invoked by default collector construction — we just verify
// the struct accepts a real-looking injected function.
func TestCPUCollector_NoRealOSCalls_WithInjectedStatFn(t *testing.T) {
	called := 0
	statFn := func(_ context.Context, _ time.Duration, _ bool) ([]float64, error) {
		called++
		return []float64{1.0}, nil
	}
	c := &CPUCollector{host: "h", statFn: statFn}
	_, _ = c.Collect(context.Background())
	if called == 0 {
		t.Error("expected injected statFn to be called")
	}
	// Real OS calls would use cpu.PercentWithContext; injected fn was used instead.
}

