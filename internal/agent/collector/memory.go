package collector

import (
	"context"
	"fmt"
	"time"

	gopsutilmem "github.com/shirou/gopsutil/v4/mem"

	"github.com/kamerrezz/theminidog/internal/model"
)

// MemoryCollector collects virtual-memory metrics using an injectable statFn
// for testability. In production, statFn is mem.VirtualMemoryWithContext.
type MemoryCollector struct {
	host   string
	statFn func(ctx context.Context) (*gopsutilmem.VirtualMemoryStat, error)
}

// NewMemoryCollector returns a MemoryCollector wired to gopsutil.
func NewMemoryCollector(host string) *MemoryCollector {
	return &MemoryCollector{
		host:   host,
		statFn: gopsutilmem.VirtualMemoryWithContext,
	}
}

// Name implements Collector.
func (c *MemoryCollector) Name() string { return "memory" }

// Collect implements Collector. It returns exactly three metrics:
// mem.used_pct, mem.used_bytes, mem.total_bytes. Labels are always nil.
// On error, it returns nil and the wrapped error.
func (c *MemoryCollector) Collect(ctx context.Context) ([]model.Metric, error) {
	now := time.Now().UTC()

	stat, err := c.statFn(ctx)
	if err != nil {
		return nil, fmt.Errorf("memory: %w", err)
	}

	return []model.Metric{
		{Time: now, Host: c.host, Name: "mem.used_pct", Value: stat.UsedPercent},
		{Time: now, Host: c.host, Name: "mem.used_bytes", Value: float64(stat.Used)},
		{Time: now, Host: c.host, Name: "mem.total_bytes", Value: float64(stat.Total)},
	}, nil
}
