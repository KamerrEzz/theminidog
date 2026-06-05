package model

import (
	"fmt"
	"math"
	"strings"
	"time"
)

var validMetricNames = map[string]struct{}{
	"cpu.usage_pct":    {},
	"mem.used_pct":     {},
	"mem.used_bytes":   {},
	"mem.total_bytes":  {},
	"disk.used_pct":    {},
	"disk.used_bytes":  {},
	"disk.total_bytes": {},
	"net.bytes_in":     {},
	"net.bytes_out":    {},
}

// Metric represents a single collected measurement.
type Metric struct {
	Time   time.Time         `json:"time"`
	Host   string            `json:"host"`
	Name   string            `json:"name"`
	Value  float64           `json:"value"`
	Labels map[string]string `json:"labels,omitempty"`
}

// MetricBatch groups a set of Metric values collected from a single host.
type MetricBatch struct {
	Host    string   `json:"host"`
	Metrics []Metric `json:"metrics"`
}

// Validate returns an error if the Metric is malformed.
func (m Metric) Validate() error {
	if strings.TrimSpace(m.Host) == "" {
		return fmt.Errorf("metric host must not be empty")
	}
	if _, ok := validMetricNames[m.Name]; !ok {
		return fmt.Errorf("unknown metric name %q", m.Name)
	}
	if m.Time.IsZero() {
		return fmt.Errorf("metric time must not be zero")
	}
	if math.IsNaN(m.Value) || math.IsInf(m.Value, 0) {
		return fmt.Errorf("metric value must be finite")
	}
	for k, v := range m.Labels {
		if k == "" || v == "" {
			return fmt.Errorf("metric label key and value must not be empty")
		}
	}
	return nil
}

// Validate returns an error if the MetricBatch is malformed.
func (b MetricBatch) Validate() error {
	if strings.TrimSpace(b.Host) == "" {
		return fmt.Errorf("batch host must not be empty")
	}
	for i, m := range b.Metrics {
		if err := m.Validate(); err != nil {
			return fmt.Errorf("metric[%d]: %w", i, err)
		}
	}
	return nil
}
