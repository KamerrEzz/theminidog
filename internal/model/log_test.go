package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func validLogEntry() LogEntry {
	return LogEntry{
		Time:    time.Now(),
		Host:    "web-01",
		Path:    "/var/log/app.log",
		Level:   LevelInfo,
		Message: "server started",
	}
}

func TestLogEntry_Validate_Valid(t *testing.T) {
	e := validLogEntry()
	if err := e.Validate(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestLogEntry_Validate_EmptyHost(t *testing.T) {
	e := validLogEntry()
	e.Host = ""
	if err := e.Validate(); err == nil {
		t.Fatal("expected error for empty host, got nil")
	}
}

func TestLogEntry_Validate_WhitespaceHost(t *testing.T) {
	e := validLogEntry()
	e.Host = "   "
	if err := e.Validate(); err == nil {
		t.Fatal("expected error for whitespace-only host, got nil")
	}
}

func TestLogEntry_Validate_EmptyMessage(t *testing.T) {
	e := validLogEntry()
	e.Message = ""
	if err := e.Validate(); err == nil {
		t.Fatal("expected error for empty message, got nil")
	}
}

func TestLogEntry_Validate_ZeroTime(t *testing.T) {
	e := validLogEntry()
	e.Time = time.Time{}
	if err := e.Validate(); err == nil {
		t.Fatal("expected error for zero time, got nil")
	}
}

func TestLogEntry_Validate_InvalidLevel(t *testing.T) {
	e := validLogEntry()
	e.Level = "critical"
	if err := e.Validate(); err == nil {
		t.Fatal("expected error for level 'critical', got nil")
	}
}

func TestLogEntry_Validate_AllValidLevels(t *testing.T) {
	levels := []LogLevel{LevelInfo, LevelWarn, LevelError, LevelDebug}
	for _, level := range levels {
		t.Run(string(level), func(t *testing.T) {
			e := validLogEntry()
			e.Level = level
			if err := e.Validate(); err != nil {
				t.Fatalf("level %q: expected nil, got %v", level, err)
			}
		})
	}
}

func TestLogEntry_Validate_LevelConstants(t *testing.T) {
	tests := []struct {
		level LogLevel
		want  string
	}{
		{LevelInfo, "info"},
		{LevelWarn, "warn"},
		{LevelError, "error"},
		{LevelDebug, "debug"},
	}
	for _, tc := range tests {
		if string(tc.level) != tc.want {
			t.Errorf("LevelConstant %v: got %q, want %q", tc.level, string(tc.level), tc.want)
		}
	}
}

func TestLogBatch_Validate_EmptyEntries(t *testing.T) {
	b := LogBatch{Host: "web-01", Entries: []LogEntry{}}
	if err := b.Validate(); err == nil {
		t.Fatal("expected error for empty entries, got nil")
	}
}

func TestLogBatch_Validate_NilEntries(t *testing.T) {
	b := LogBatch{Host: "web-01", Entries: nil}
	if err := b.Validate(); err == nil {
		t.Fatal("expected error for nil entries, got nil")
	}
}

func TestLogBatch_Validate_OversizedBatch(t *testing.T) {
	entries := make([]LogEntry, 1001)
	for i := range entries {
		entries[i] = validLogEntry()
	}
	b := LogBatch{Host: "web-01", Entries: entries}
	if err := b.Validate(); err == nil {
		t.Fatal("expected error for 1001 entries, got nil")
	}
}

func TestLogBatch_Validate_MaxSizeBatch(t *testing.T) {
	entries := make([]LogEntry, 1000)
	for i := range entries {
		entries[i] = validLogEntry()
	}
	b := LogBatch{Host: "web-01", Entries: entries}
	if err := b.Validate(); err != nil {
		t.Fatalf("expected nil for 1000 entries, got %v", err)
	}
}

func TestLogBatch_Validate_OneValidEntry(t *testing.T) {
	b := LogBatch{Host: "web-01", Entries: []LogEntry{validLogEntry()}}
	if err := b.Validate(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestLogBatch_Validate_EntryWithInvalidLevel(t *testing.T) {
	e := validLogEntry()
	e.Level = "critical"
	b := LogBatch{Host: "web-01", Entries: []LogEntry{e}}
	err := b.Validate()
	if err == nil {
		t.Fatal("expected error for entry with invalid level, got nil")
	}
	// Error must include index prefix
	if !strings.Contains(err.Error(), "entry[0]") {
		t.Errorf("expected error to contain 'entry[0]', got: %v", err)
	}
}

func TestLogBatch_Validate_EmptyHost(t *testing.T) {
	b := LogBatch{Host: "", Entries: []LogEntry{validLogEntry()}}
	if err := b.Validate(); err == nil {
		t.Fatal("expected error for empty batch host, got nil")
	}
}

func TestLogBatch_Validate_IndexPrefixOnSecondEntry(t *testing.T) {
	good := validLogEntry()
	bad := validLogEntry()
	bad.Level = "panic"
	b := LogBatch{Host: "web-01", Entries: []LogEntry{good, bad}}
	err := b.Validate()
	if err == nil {
		t.Fatal("expected error for second entry invalid level, got nil")
	}
	if !strings.Contains(err.Error(), "entry[1]") {
		t.Errorf("expected error to contain 'entry[1]', got: %v", err)
	}
}

func TestLogEntry_JSONRoundTrip(t *testing.T) {
	original := LogEntry{
		Time:    time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
		Host:    "db-01",
		Path:    "/var/log/postgres.log",
		Level:   LevelError,
		Message: "connection refused",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded LogEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Host != original.Host {
		t.Errorf("Host: got %q, want %q", decoded.Host, original.Host)
	}
	if decoded.Path != original.Path {
		t.Errorf("Path: got %q, want %q", decoded.Path, original.Path)
	}
	if decoded.Level != original.Level {
		t.Errorf("Level: got %q, want %q", decoded.Level, original.Level)
	}
	if decoded.Message != original.Message {
		t.Errorf("Message: got %q, want %q", decoded.Message, original.Message)
	}
	if !decoded.Time.Equal(original.Time) {
		t.Errorf("Time: got %v, want %v", decoded.Time, original.Time)
	}

	// Verify JSON keys are correct
	jsonStr := string(data)
	for _, key := range []string{`"time"`, `"host"`, `"path"`, `"level"`, `"message"`} {
		if !strings.Contains(jsonStr, key) {
			t.Errorf("JSON missing key %q in %s", key, jsonStr)
		}
	}
}

func TestLogBatch_JSONRoundTrip(t *testing.T) {
	original := LogBatch{
		Host: "web-01",
		Entries: []LogEntry{
			{
				Time:    time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC),
				Host:    "web-01",
				Path:    "/var/log/nginx.log",
				Level:   LevelWarn,
				Message: "slow query",
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded LogBatch
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Host != original.Host {
		t.Errorf("Host: got %q, want %q", decoded.Host, original.Host)
	}
	if len(decoded.Entries) != len(original.Entries) {
		t.Fatalf("Entries length: got %d, want %d", len(decoded.Entries), len(original.Entries))
	}
	if decoded.Entries[0].Message != original.Entries[0].Message {
		t.Errorf("Entry[0].Message: got %q, want %q", decoded.Entries[0].Message, original.Entries[0].Message)
	}

	// Verify JSON top-level keys
	jsonStr := string(data)
	for _, key := range []string{`"host"`, `"entries"`} {
		if !strings.Contains(jsonStr, key) {
			t.Errorf("JSON missing key %q in %s", key, jsonStr)
		}
	}
}

// Ensure the error message format matches the spec
func TestLogEntry_Validate_ErrorMessages(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*LogEntry)
		wantMsg string
	}{
		{"empty host", func(e *LogEntry) { e.Host = "" }, "host"},
		{"empty message", func(e *LogEntry) { e.Message = "" }, "message"},
		{"zero time", func(e *LogEntry) { e.Time = time.Time{} }, "time"},
		{"invalid level", func(e *LogEntry) { e.Level = "trace" }, "level"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := validLogEntry()
			tc.modify(&e)
			err := e.Validate()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.wantMsg) {
				t.Errorf("expected error message to contain %q, got: %v", tc.wantMsg, err)
			}
		})
	}
}

func TestLogBatch_Validate_ErrorMessages(t *testing.T) {
	tests := []struct {
		name    string
		batch   LogBatch
		wantMsg string
	}{
		{"empty host", LogBatch{Host: "", Entries: []LogEntry{validLogEntry()}}, "host"},
		{"empty entries", LogBatch{Host: "x", Entries: []LogEntry{}}, "entries"},
		{"oversized", LogBatch{Host: "x", Entries: make([]LogEntry, 1001)}, "1000"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Fill oversized batch with valid entries to reach the size check
			if len(tc.batch.Entries) == 1001 {
				for i := range tc.batch.Entries {
					tc.batch.Entries[i] = validLogEntry()
				}
			}
			err := tc.batch.Validate()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("expected error message to contain %q, got: %v", tc.wantMsg, err)
			}
		})
	}
}

// Verify fmt.Errorf wrapping works correctly for entry index errors
func TestLogBatch_Validate_WrapError(t *testing.T) {
	e := validLogEntry()
	e.Host = "" // LogEntry.Validate will fail
	b := LogBatch{Host: "web-01", Entries: []LogEntry{e}}
	err := b.Validate()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	expected := fmt.Sprintf("entry[%d]", 0)
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected wrapped error with %q, got: %v", expected, err)
	}
}
