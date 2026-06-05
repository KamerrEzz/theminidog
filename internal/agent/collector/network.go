package collector

import (
	"context"
	"fmt"
	"time"

	gopsutilnet "github.com/shirou/gopsutil/v4/net"

	"github.com/kamerrezz/theminidog/internal/model"
)

// NetworkCollector collects per-interface network I/O deltas (bytes in/out).
// It uses delta semantics: the first Collect call seeds the previous-state
// snapshot and returns an empty slice; subsequent calls compute the delta
// between the current and previous snapshots and return net.bytes_in /
// net.bytes_out metrics per interface.
//
// Loopback ("lo") is always excluded. Negative deltas (counter wrap/reset)
// are clamped to zero. ioFn is injectable for testability.
type NetworkCollector struct {
	host   string
	ifaces []string
	ioFn   func(ctx context.Context, pernic bool) ([]gopsutilnet.IOCountersStat, error)
	prev   map[string]gopsutilnet.IOCountersStat
	prevAt time.Time
}

// NewNetworkCollector returns a NetworkCollector wired to gopsutil.
// Pass a non-empty ifaces slice to restrict collection to specific interfaces.
func NewNetworkCollector(host string, ifaces []string) *NetworkCollector {
	return &NetworkCollector{
		host:   host,
		ifaces: ifaces,
		ioFn:   gopsutilnet.IOCountersWithContext,
		prev:   nil,
	}
}

// Name implements Collector.
func (c *NetworkCollector) Name() string { return "network" }

// Collect implements Collector. On the first call it seeds prev and returns
// nil (no metrics). On subsequent calls it computes byte deltas per interface
// and returns net.bytes_in and net.bytes_out metrics.
func (c *NetworkCollector) Collect(ctx context.Context) ([]model.Metric, error) {
	now := time.Now().UTC()

	stats, err := c.ioFn(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("network io: %w", err)
	}

	// Build current snapshot, applying loopback and allow-list filters.
	curr := make(map[string]gopsutilnet.IOCountersStat, len(stats))
	for _, s := range stats {
		if s.Name == "lo" {
			continue
		}
		if len(c.ifaces) > 0 && !containsIface(c.ifaces, s.Name) {
			continue
		}
		curr[s.Name] = s
	}

	// First call: seed prev and return empty slice.
	if c.prev == nil {
		c.prev = curr
		c.prevAt = now
		return nil, nil
	}

	// Compute deltas.
	var metrics []model.Metric
	for name, cs := range curr {
		ps, ok := c.prev[name]
		if !ok {
			// New interface appeared since last tick; skip this cycle.
			continue
		}

		bytesIn := int64(cs.BytesRecv) - int64(ps.BytesRecv)
		bytesOut := int64(cs.BytesSent) - int64(ps.BytesSent)
		if bytesIn < 0 {
			bytesIn = 0
		}
		if bytesOut < 0 {
			bytesOut = 0
		}

		labels := map[string]string{"iface": name}
		metrics = append(metrics,
			model.Metric{Time: now, Host: c.host, Name: "net.bytes_in", Value: float64(bytesIn), Labels: labels},
			model.Metric{Time: now, Host: c.host, Name: "net.bytes_out", Value: float64(bytesOut), Labels: labels},
		)
	}

	c.prev = curr
	c.prevAt = now
	return metrics, nil
}

// containsIface reports whether name is present in the ifaces slice.
func containsIface(ifaces []string, name string) bool {
	for _, i := range ifaces {
		if i == name {
			return true
		}
	}
	return false
}
