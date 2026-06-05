package logtail

import (
	"testing"
)

var parseTests = []struct {
	line string
	want string
}{
	// JSON format — "level" key
	{`{"level":"error","msg":"oops"}`, "error"},
	{`{"level":"WARNING","msg":"slow"}`, "warn"},
	{`{"level":"debug","msg":"cache miss"}`, "debug"},
	{`{"level":"info","msg":"started"}`, "info"},
	// JSON format — "severity" key fallback
	{`{"severity":"fatal","msg":"crash"}`, "error"},
	{`{"severity":"CRITICAL","msg":"disk full"}`, "error"},
	// Bracketed format
	{`[ERROR] database connection failed`, "error"},
	{`[warn] slow query detected`, "warn"},
	{`[DEBUG] cache miss`, "debug"},
	{`[INFO] server started`, "info"},
	// Prefixed format (LEVEL:)
	{`ERROR: disk full`, "error"},
	{`WARN: high memory usage`, "warn"},
	{`INFO: server started`, "info"},
	{`DEBUG: request received`, "debug"},
	// Timestamp + level
	{`2024-01-01T12:00:00Z WARN timeout exceeded`, "warn"},
	// Aliases — normalization
	{`err: connection refused`, "error"},
	{`fatal: out of memory`, "error"},
	{`critical: disk full`, "error"},
	{`warning: low disk space`, "warn"},
	{`dbg: request received`, "debug"},
	{`trace: entering function`, "debug"},
	// Default fallback
	{`something happened without a level`, "info"},
	{``, "info"},
}

func TestParseLevel(t *testing.T) {
	for _, tc := range parseTests {
		t.Run(tc.line, func(t *testing.T) {
			got := ParseLevel(tc.line)
			if got != tc.want {
				t.Errorf("ParseLevel(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

// Verify ParseLevel is a pure function — calling it twice returns the same result
func TestParseLevel_Idempotent(t *testing.T) {
	line := `{"level":"error","msg":"test"}`
	first := ParseLevel(line)
	second := ParseLevel(line)
	if first != second {
		t.Errorf("ParseLevel not idempotent: first=%q, second=%q", first, second)
	}
}

// Verify the empty string returns exactly "info"
func TestParseLevel_EmptyReturnsInfo(t *testing.T) {
	got := ParseLevel("")
	if got != "info" {
		t.Errorf("ParseLevel(\"\") = %q, want %q", got, "info")
	}
}

// Verify warning normalization
func TestParseLevel_WarningNormalization(t *testing.T) {
	cases := []string{
		`{"level":"warning","msg":"x"}`,
		`[WARNING] test`,
		`WARNING: test`,
	}
	for _, c := range cases {
		got := ParseLevel(c)
		if got != "warn" {
			t.Errorf("ParseLevel(%q) = %q, want %q", c, got, "warn")
		}
	}
}

// Verify fatal/panic/critical all map to "error"
func TestParseLevel_FatalAliases(t *testing.T) {
	cases := []string{
		`[FATAL] crash`,
		`[PANIC] crash`,
		`[CRITICAL] disk full`,
	}
	for _, c := range cases {
		got := ParseLevel(c)
		if got != "error" {
			t.Errorf("ParseLevel(%q) = %q, want %q", c, got, "error")
		}
	}
}
