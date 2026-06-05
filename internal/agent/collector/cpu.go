package collector

import (
	"context"
	"fmt"
	"strconv"
	"time"

	gopsutilcpu "github.com/shirou/gopsutil/v4/cpu"

	"github.com/kamerrezz/theminidog/internal/model"
)

// CPUCollector collects CPU usage metrics using an injectable statFn for
// testability. In production, statFn is cpu.PercentWithContext from gopsutil.
type CPUCollector struct {
	host   string
	statFn func(ctx context.Context, interval time.Duration, percpu bool) ([]float64, error)
}

// NewCPUCollector returns a CPUCollector wired to gopsutil for real OS data.
func NewCPUCollector(host string) *CPUCollector {
	return &CPUCollector{
		host:   host,
		statFn: gopsutilcpu.PercentWithContext,
	}
}

// Name implements Collector.
func (c *CPUCollector) Name() string { return "cpu" }

// Collect implements Collector. It returns one aggregate cpu.usage_pct metric
// with label core=total, plus one per logical core with label core=<index>.
// On any error, it returns nil and the wrapped error (no partial results).
func (c *CPUCollector) Collect(ctx context.Context) ([]model.Metric, error) {
	now := time.Now().UTC()

	// Aggregate (percpu=false)
	totals, err := c.statFn(ctx, 0, false)
	if err != nil {
		return nil, fmt.Errorf("cpu aggregate: %w", err)
	}
	if len(totals) == 0 {
		return nil, fmt.Errorf("cpu aggregate: empty result")
	}

	// Per-core (percpu=true)
	perCore, err := c.statFn(ctx, 0, true)
	if err != nil {
		return nil, fmt.Errorf("cpu per-core: %w", err)
	}

	metrics := make([]model.Metric, 0, 1+len(perCore))

	// Total metric
	metrics = append(metrics, model.Metric{
		Time:   now,
		Host:   c.host,
		Name:   "cpu.usage_pct",
		Value:  totals[0],
		Labels: map[string]string{"core": "total"},
	})

	// Per-core metrics
	for i, v := range perCore {
		metrics = append(metrics, model.Metric{
			Time:   now,
			Host:   c.host,
			Name:   "cpu.usage_pct",
			Value:  v,
			Labels: map[string]string{"core": strconv.Itoa(i)},
		})
	}

	return metrics, nil
}
