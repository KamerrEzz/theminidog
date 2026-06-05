package collector

import (
	"context"
	"errors"
	"testing"

	gopsutildisk "github.com/shirou/gopsutil/v4/disk"
)

// ── DiskCollector.Name ────────────────────────────────────────────────────────

func TestDiskCollector_Name(t *testing.T) {
	c := NewDiskCollector("host1", nil)
	if c.Name() != "disk" {
		t.Errorf("expected Name()=disk, got %q", c.Name())
	}
}

// ── DiskCollector.Collect — one error partition is skipped ───────────────────

func TestDiskCollector_Collect_SkipsErrorPartition_ReturnsOthers(t *testing.T) {
	// "/" succeeds, "/mnt/data" fails — expect 3 metrics, nil error
	partFn := func(_ context.Context, _ bool) ([]gopsutildisk.PartitionStat, error) {
		return []gopsutildisk.PartitionStat{
			{Mountpoint: "/"},
			{Mountpoint: "/mnt/data"},
		}, nil
	}
	usageFn := func(_ context.Context, path string) (*gopsutildisk.UsageStat, error) {
		if path == "/" {
			return &gopsutildisk.UsageStat{
				Total:       100_000_000,
				Used:        40_000_000,
				UsedPercent: 40.0,
			}, nil
		}
		return nil, errors.New("permission denied")
	}

	c := &DiskCollector{host: "h", partFn: partFn, usageFn: usageFn}
	metrics, err := c.Collect(context.Background())

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if len(metrics) != 3 {
		t.Fatalf("expected 3 metrics for '/', got %d", len(metrics))
	}
	for _, m := range metrics {
		if m.Labels["mount"] != "/" {
			t.Errorf("expected label mount=/, got %q", m.Labels["mount"])
		}
	}
}

// ── DiskCollector.Collect — two ok partitions → 6 metrics ────────────────────

func TestDiskCollector_Collect_TwoPartitions_ReturnsSixMetrics(t *testing.T) {
	partFn := func(_ context.Context, _ bool) ([]gopsutildisk.PartitionStat, error) {
		return []gopsutildisk.PartitionStat{
			{Mountpoint: "/"},
			{Mountpoint: "/home"},
		}, nil
	}
	usageFn := func(_ context.Context, path string) (*gopsutildisk.UsageStat, error) {
		return &gopsutildisk.UsageStat{Total: 1_000_000, Used: 500_000, UsedPercent: 50.0}, nil
	}

	c := &DiskCollector{host: "h", partFn: partFn, usageFn: usageFn}
	metrics, err := c.Collect(context.Background())

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if len(metrics) != 6 {
		t.Fatalf("expected 6 metrics (3 per mount), got %d", len(metrics))
	}
}

// ── DiskCollector.Collect — partFn error ─────────────────────────────────────

func TestDiskCollector_Collect_PartFnError_ReturnsNilAndError(t *testing.T) {
	want := errors.New("partition enumeration failed")
	partFn := func(_ context.Context, _ bool) ([]gopsutildisk.PartitionStat, error) {
		return nil, want
	}
	c := &DiskCollector{host: "h", partFn: partFn, usageFn: nil}

	metrics, err := c.Collect(context.Background())
	if metrics != nil {
		t.Errorf("expected nil metrics on partFn error, got %v", metrics)
	}
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, want) {
		t.Errorf("expected wrapped error %v, got %v", want, err)
	}
}

// ── DiskCollector.Collect — Total==0 partition skipped ───────────────────────

func TestDiskCollector_Collect_ZeroTotalPartition_Skipped(t *testing.T) {
	partFn := func(_ context.Context, _ bool) ([]gopsutildisk.PartitionStat, error) {
		return []gopsutildisk.PartitionStat{
			{Mountpoint: "/zero"},
		}, nil
	}
	usageFn := func(_ context.Context, _ string) (*gopsutildisk.UsageStat, error) {
		return &gopsutildisk.UsageStat{Total: 0, Used: 0, UsedPercent: 0}, nil
	}

	c := &DiskCollector{host: "h", partFn: partFn, usageFn: usageFn}
	metrics, err := c.Collect(context.Background())

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if len(metrics) != 0 {
		t.Errorf("expected 0 metrics for zero-total partition, got %d", len(metrics))
	}
}

// ── DiskCollector.Collect — label key is exactly "mount" ─────────────────────

func TestDiskCollector_Collect_LabelKeyIsMountExact(t *testing.T) {
	partFn := func(_ context.Context, _ bool) ([]gopsutildisk.PartitionStat, error) {
		return []gopsutildisk.PartitionStat{{Mountpoint: "/data"}}, nil
	}
	usageFn := func(_ context.Context, _ string) (*gopsutildisk.UsageStat, error) {
		return &gopsutildisk.UsageStat{Total: 100, Used: 50, UsedPercent: 50.0}, nil
	}

	c := &DiskCollector{host: "h", partFn: partFn, usageFn: usageFn}
	metrics, _ := c.Collect(context.Background())

	for i, m := range metrics {
		if _, ok := m.Labels["mount"]; !ok {
			t.Errorf("metric[%d]: missing label key 'mount'", i)
		}
		if len(m.Labels) != 1 {
			t.Errorf("metric[%d]: expected exactly 1 label, got %d: %v", i, len(m.Labels), m.Labels)
		}
		if m.Labels["mount"] != "/data" {
			t.Errorf("metric[%d]: expected mount=/data, got %q", i, m.Labels["mount"])
		}
	}
}

// ── DiskCollector.Collect — mounts allow-list (partFn not called) ─────────────

func TestDiskCollector_Collect_MountsAllowList_PartFnNotCalled(t *testing.T) {
	partFnCalled := false
	partFn := func(_ context.Context, _ bool) ([]gopsutildisk.PartitionStat, error) {
		partFnCalled = true
		return nil, errors.New("should not be called")
	}
	usageFn := func(_ context.Context, path string) (*gopsutildisk.UsageStat, error) {
		return &gopsutildisk.UsageStat{Total: 100, Used: 50, UsedPercent: 50.0}, nil
	}

	c := &DiskCollector{
		host:    "h",
		mounts:  []string{"/"},
		partFn:  partFn,
		usageFn: usageFn,
	}
	metrics, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if partFnCalled {
		t.Error("partFn must NOT be called when mounts allow-list is provided")
	}
	if len(metrics) != 3 {
		t.Errorf("expected 3 metrics for mount '/', got %d", len(metrics))
	}
}

// ── DiskCollector.Collect — metric names are exact ───────────────────────────

func TestDiskCollector_Collect_ExactMetricNames(t *testing.T) {
	partFn := func(_ context.Context, _ bool) ([]gopsutildisk.PartitionStat, error) {
		return []gopsutildisk.PartitionStat{{Mountpoint: "/"}}, nil
	}
	usageFn := func(_ context.Context, _ string) (*gopsutildisk.UsageStat, error) {
		return &gopsutildisk.UsageStat{Total: 100, Used: 50, UsedPercent: 50.0}, nil
	}

	c := &DiskCollector{host: "h", partFn: partFn, usageFn: usageFn}
	metrics, _ := c.Collect(context.Background())

	expected := map[string]bool{
		"disk.used_pct":    true,
		"disk.used_bytes":  true,
		"disk.total_bytes": true,
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
