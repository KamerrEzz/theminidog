package collector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kamerrezz/theminidog/internal/model"
)

// ── stub collectors ──────────────────────────────────────────────────────────

type stubCollector struct {
	name    string
	metrics []model.Metric
	err     error
}

func (s *stubCollector) Name() string { return s.name }
func (s *stubCollector) Collect(_ context.Context) ([]model.Metric, error) {
	return s.metrics, s.err
}

func makeMetric(name string) model.Metric {
	return model.Metric{
		Time:  time.Now(),
		Host:  "test-host",
		Name:  name,
		Value: 1.0,
	}
}

// ── Registry.Add ─────────────────────────────────────────────────────────────

func TestRegistry_Add_AppendsCollector(t *testing.T) {
	r := NewRegistry()
	if len(r.collectors) != 0 {
		t.Fatalf("expected empty registry, got %d collectors", len(r.collectors))
	}
	c := &stubCollector{name: "cpu"}
	r.Add(c)
	if len(r.collectors) != 1 {
		t.Fatalf("expected 1 collector after Add, got %d", len(r.collectors))
	}
	if r.collectors[0].Name() != "cpu" {
		t.Errorf("expected collector name=cpu, got %q", r.collectors[0].Name())
	}
}

func TestRegistry_Add_MultipleCollectors(t *testing.T) {
	r := NewRegistry(
		&stubCollector{name: "cpu"},
		&stubCollector{name: "mem"},
	)
	r.Add(&stubCollector{name: "disk"})
	if len(r.collectors) != 3 {
		t.Fatalf("expected 3 collectors, got %d", len(r.collectors))
	}
}

// ── Registry.CollectAll ──────────────────────────────────────────────────────

func TestRegistry_CollectAll_EmptyRegistry_NoMetricsNoErrors(t *testing.T) {
	r := NewRegistry()
	metrics, errs := r.CollectAll(context.Background())
	if len(metrics) != 0 {
		t.Errorf("expected 0 metrics from empty registry, got %d", len(metrics))
	}
	if len(errs) != 0 {
		t.Errorf("expected 0 errors from empty registry, got %d", len(errs))
	}
}

func TestRegistry_CollectAll_AllOK_ReturnsAllMetrics(t *testing.T) {
	cpuMetric := makeMetric("cpu.usage_pct")
	memMetric := makeMetric("mem.used_pct")

	r := NewRegistry(
		&stubCollector{name: "cpu", metrics: []model.Metric{cpuMetric}},
		&stubCollector{name: "mem", metrics: []model.Metric{memMetric}},
	)

	metrics, errs := r.CollectAll(context.Background())
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
	}
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(metrics))
	}
}

func TestRegistry_CollectAll_OneError_ReturnsPartialMetrics(t *testing.T) {
	cpuMetric := makeMetric("cpu.usage_pct")
	collectorErr := errors.New("disk read failed")

	r := NewRegistry(
		&stubCollector{name: "cpu", metrics: []model.Metric{cpuMetric}},
		&stubCollector{name: "disk", err: collectorErr},
	)

	metrics, errs := r.CollectAll(context.Background())

	// Partial metrics: cpu succeeded
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric (from cpu), got %d", len(metrics))
	}
	if metrics[0].Name != "cpu.usage_pct" {
		t.Errorf("expected cpu.usage_pct metric, got %q", metrics[0].Name)
	}

	// One error from disk
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	// Error should be wrapped with collector name prefix
	errMsg := errs[0].Error()
	if len(errMsg) == 0 {
		t.Fatal("error message must not be empty")
	}
	// Check that the collector name is part of the error context
	if !containsSubstr(errMsg, "disk") {
		t.Errorf("expected error to contain collector name 'disk', got: %q", errMsg)
	}
}

func TestRegistry_CollectAll_AllError_ReturnsNoMetricsAndAllErrors(t *testing.T) {
	err1 := errors.New("cpu failure")
	err2 := errors.New("mem failure")

	r := NewRegistry(
		&stubCollector{name: "cpu", err: err1},
		&stubCollector{name: "mem", err: err2},
	)

	metrics, errs := r.CollectAll(context.Background())
	if len(metrics) != 0 {
		t.Errorf("expected 0 metrics when all collectors fail, got %d", len(metrics))
	}
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errs))
	}
}

func TestRegistry_CollectAll_MultipleMetricsPerCollector(t *testing.T) {
	m1 := makeMetric("cpu.usage_pct")
	m2 := makeMetric("cpu.usage_pct")
	m3 := makeMetric("mem.used_pct")

	r := NewRegistry(
		&stubCollector{name: "cpu", metrics: []model.Metric{m1, m2}},
		&stubCollector{name: "mem", metrics: []model.Metric{m3}},
	)

	metrics, errs := r.CollectAll(context.Background())
	if len(errs) != 0 {
		t.Errorf("expected 0 errors, got %d", len(errs))
	}
	if len(metrics) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(metrics))
	}
}

// containsSubstr is a simple substring check to avoid importing strings in tests.
func containsSubstr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
