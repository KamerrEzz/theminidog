package model

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

// helper to build a valid metric
func validMetric() Metric {
	return Metric{
		Time:  time.Now(),
		Host:  "test-host",
		Name:  "cpu.usage_pct",
		Value: 42.0,
	}
}

// ── Metric.Validate ─────────────────────────────────────────────────────────

func TestMetric_Validate_ValidMetricPasses(t *testing.T) {
	m := validMetric()
	if err := m.Validate(); err != nil {
		t.Fatalf("expected nil error for valid metric, got: %v", err)
	}
}

func TestMetric_Validate_EmptyHostFails(t *testing.T) {
	m := validMetric()
	m.Host = ""
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for empty host, got nil")
	}
}

func TestMetric_Validate_WhitespaceHostFails(t *testing.T) {
	m := validMetric()
	m.Host = "   "
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for whitespace-only host, got nil")
	}
}

func TestMetric_Validate_AllCanonicalNamesPass(t *testing.T) {
	names := []string{
		"cpu.usage_pct",
		"mem.used_pct",
		"mem.used_bytes",
		"mem.total_bytes",
		"disk.used_pct",
		"disk.used_bytes",
		"disk.total_bytes",
		"net.bytes_in",
		"net.bytes_out",
	}
	for _, name := range names {
		m := validMetric()
		m.Name = name
		if err := m.Validate(); err != nil {
			t.Errorf("expected %q to pass validation, got: %v", name, err)
		}
	}
}

func TestMetric_Validate_NonCanonicalNameFails(t *testing.T) {
	m := validMetric()
	m.Name = "cpu.temperature"
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for non-canonical name, got nil")
	}
}

func TestMetric_Validate_UnknownNameFails(t *testing.T) {
	m := validMetric()
	m.Name = "unknown.metric"
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for unknown metric name, got nil")
	}
}

func TestMetric_Validate_ZeroTimeFails(t *testing.T) {
	m := validMetric()
	m.Time = time.Time{} // zero value
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for zero time, got nil")
	}
}

func TestMetric_Validate_NaNValueFails(t *testing.T) {
	m := validMetric()
	m.Value = math.NaN()
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for NaN value, got nil")
	}
}

func TestMetric_Validate_PosInfValueFails(t *testing.T) {
	m := validMetric()
	m.Value = math.Inf(1)
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for +Inf value, got nil")
	}
}

func TestMetric_Validate_NegInfValueFails(t *testing.T) {
	m := validMetric()
	m.Value = math.Inf(-1)
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for -Inf value, got nil")
	}
}

func TestMetric_Validate_EmptyLabelKeyFails(t *testing.T) {
	m := validMetric()
	m.Labels = map[string]string{"": "some-value"}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for empty label key, got nil")
	}
}

func TestMetric_Validate_EmptyLabelValueFails(t *testing.T) {
	m := validMetric()
	m.Labels = map[string]string{"core": ""}
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error for empty label value, got nil")
	}
}

func TestMetric_Validate_ValidLabelsPass(t *testing.T) {
	m := validMetric()
	m.Labels = map[string]string{"core": "total", "region": "us-east"}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected nil error for valid labels, got: %v", err)
	}
}

// ── JSON round-trip ──────────────────────────────────────────────────────────

func TestMetric_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	original := Metric{
		Time:   now,
		Host:   "round-trip-host",
		Name:   "disk.used_pct",
		Value:  73.5,
		Labels: map[string]string{"mount": "/"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored Metric
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if !restored.Time.Equal(original.Time) {
		t.Errorf("Time mismatch: got %v, want %v", restored.Time, original.Time)
	}
	if restored.Host != original.Host {
		t.Errorf("Host mismatch: got %q, want %q", restored.Host, original.Host)
	}
	if restored.Name != original.Name {
		t.Errorf("Name mismatch: got %q, want %q", restored.Name, original.Name)
	}
	if restored.Value != original.Value {
		t.Errorf("Value mismatch: got %v, want %v", restored.Value, original.Value)
	}
	if len(restored.Labels) != len(original.Labels) {
		t.Errorf("Labels length mismatch: got %d, want %d", len(restored.Labels), len(original.Labels))
	}
	for k, v := range original.Labels {
		if restored.Labels[k] != v {
			t.Errorf("Label[%q] mismatch: got %q, want %q", k, restored.Labels[k], v)
		}
	}
}

// ── MetricBatch.Validate ─────────────────────────────────────────────────────

func TestMetricBatch_Validate_ValidBatchPasses(t *testing.T) {
	b := MetricBatch{
		Host:    "test-host",
		Metrics: []Metric{validMetric()},
	}
	if err := b.Validate(); err != nil {
		t.Fatalf("expected nil error for valid batch, got: %v", err)
	}
}

func TestMetricBatch_Validate_EmptyHostFails(t *testing.T) {
	b := MetricBatch{
		Host:    "",
		Metrics: []Metric{validMetric()},
	}
	err := b.Validate()
	if err == nil {
		t.Fatal("expected error for empty batch host, got nil")
	}
}

func TestMetricBatch_Validate_PropagatesMetricErrorWithIndex(t *testing.T) {
	bad := validMetric()
	bad.Host = "" // invalid

	b := MetricBatch{
		Host:    "batch-host",
		Metrics: []Metric{validMetric(), bad},
	}
	err := b.Validate()
	if err == nil {
		t.Fatal("expected error propagated from invalid metric, got nil")
	}
	// The error message must contain the index prefix "metric[1]"
	errMsg := err.Error()
	if len(errMsg) == 0 {
		t.Fatal("error message must not be empty")
	}
	// We rely on fmt.Errorf("metric[%d]: %w", i, err) wrapping
	want := "metric[1]"
	if !containsStr(errMsg, want) {
		t.Errorf("expected error to contain %q, got: %q", want, errMsg)
	}
}

func TestMetricBatch_Validate_MultipleMetricsAllValid(t *testing.T) {
	m1 := validMetric()
	m2 := validMetric()
	m2.Name = "mem.used_pct"
	m2.Value = 55.0

	b := MetricBatch{
		Host:    "multi-host",
		Metrics: []Metric{m1, m2},
	}
	if err := b.Validate(); err != nil {
		t.Fatalf("expected nil error for multiple valid metrics, got: %v", err)
	}
}

// containsStr is a simple substring check to avoid importing strings in tests.
func containsStr(s, sub string) bool {
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
