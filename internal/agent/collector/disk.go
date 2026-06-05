package collector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	gopsutildisk "github.com/shirou/gopsutil/v4/disk"

	"github.com/kamerrezz/theminidog/internal/model"
)

// DiskCollector collects per-mount-point disk usage metrics. When mounts is
// non-empty only those paths are queried; otherwise all physical partitions are
// enumerated via partFn. Partitions whose usage query fails are skipped (logged
// at Warn level). partFn and usageFn are injectable for testing.
type DiskCollector struct {
	host    string
	mounts  []string
	partFn  func(ctx context.Context, all bool) ([]gopsutildisk.PartitionStat, error)
	usageFn func(ctx context.Context, path string) (*gopsutildisk.UsageStat, error)
}

// NewDiskCollector returns a DiskCollector wired to gopsutil.
// Pass a non-empty mounts slice to restrict collection to specific mount points.
func NewDiskCollector(host string, mounts []string) *DiskCollector {
	return &DiskCollector{
		host:    host,
		mounts:  mounts,
		partFn:  gopsutildisk.PartitionsWithContext,
		usageFn: gopsutildisk.UsageWithContext,
	}
}

// Name implements Collector.
func (c *DiskCollector) Name() string { return "disk" }

// Collect implements Collector. For each mount point it emits disk.used_pct,
// disk.used_bytes, and disk.total_bytes with label {"mount": <path>}.
// Partitions that fail usage queries are logged and skipped — they do not
// cause the entire collection to fail. An error is returned only when partition
// enumeration itself fails.
func (c *DiskCollector) Collect(ctx context.Context) ([]model.Metric, error) {
	now := time.Now().UTC()

	// Determine which mount points to query.
	var paths []string
	if len(c.mounts) > 0 {
		paths = c.mounts
	} else {
		parts, err := c.partFn(ctx, false)
		if err != nil {
			return nil, fmt.Errorf("disk partitions: %w", err)
		}
		for _, p := range parts {
			paths = append(paths, p.Mountpoint)
		}
	}

	var metrics []model.Metric
	for _, path := range paths {
		usage, err := c.usageFn(ctx, path)
		if err != nil {
			slog.Warn("disk usage error, skipping mount", "mount", path, "err", err)
			continue
		}
		if usage.Total == 0 {
			continue
		}
		labels := map[string]string{"mount": path}
		metrics = append(metrics,
			model.Metric{Time: now, Host: c.host, Name: "disk.used_pct", Value: usage.UsedPercent, Labels: labels},
			model.Metric{Time: now, Host: c.host, Name: "disk.used_bytes", Value: float64(usage.Used), Labels: labels},
			model.Metric{Time: now, Host: c.host, Name: "disk.total_bytes", Value: float64(usage.Total), Labels: labels},
		)
	}
	return metrics, nil
}
