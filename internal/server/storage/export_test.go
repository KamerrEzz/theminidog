package storage

import "time"

// SetNowFn injects a custom time function into HostTracker for deterministic tests.
func SetNowFn(t *HostTracker, fn func() time.Time) { t.nowFn = fn }

// ScanOnce runs a single scan cycle on the HostTracker for testing.
func ScanOnce(t *HostTracker) { t.scan() }
