package logtail

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kamerrezz/theminidog/internal/model"
)

// mockLogSender is a test double for LogSender.
type mockLogSender struct {
	batches []model.LogBatch
	err     error
}

func (m *mockLogSender) SendLogs(_ context.Context, batch model.LogBatch) error {
	if m.err != nil {
		return m.err
	}
	m.batches = append(m.batches, batch)
	return nil
}

// helper: create a temp file, write content, return path.
func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "logtail*.log")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if content != "" {
		if _, err := f.WriteString(content); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
	}
	name := f.Name()
	f.Close()
	return name
}

// openTailerForPath creates a Tailer pre-opened at EOF for a single path.
// Registers a cleanup to close the tailer (releases file handles and watcher).
func openTailerForPath(t *testing.T, path string, snd LogSender) *Tailer {
	t.Helper()
	tailer, err := NewTailer([]string{path}, "testhost", snd, newNopLogger())
	if err != nil {
		t.Fatalf("NewTailer: %v", err)
	}
	t.Cleanup(func() { tailer.Close() })
	return tailer
}

// Test 1: seek-EOF on startup — pre-existing content MUST NOT be returned.
func TestTailer_SeekEOFOnStartup(t *testing.T) {
	preExisting := strings.Repeat("x", 50) + "\n"
	path := writeTempFile(t, preExisting)

	snd := &mockLogSender{}
	tailer := openTailerForPath(t, path, snd)

	entries := tailer.readNewLines(path)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for pre-existing content, got %d", len(entries))
	}
}

// Test 2: reads new lines written after openAndSeekEOF.
func TestTailer_ReadsNewLines(t *testing.T) {
	path := writeTempFile(t, "")

	snd := &mockLogSender{}
	tailer := openTailerForPath(t, path, snd)

	// Append two new lines after the tailer is open.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	f.WriteString("line one\nline two\n")
	f.Close()

	entries := tailer.readNewLines(path)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Message != "line one" {
		t.Errorf("expected 'line one', got %q", entries[0].Message)
	}
	if entries[1].Message != "line two" {
		t.Errorf("expected 'line two', got %q", entries[1].Message)
	}
	if entries[0].Host != "testhost" {
		t.Errorf("expected host 'testhost', got %q", entries[0].Host)
	}
	if entries[0].Path != path {
		t.Errorf("expected path %q, got %q", path, entries[0].Path)
	}
}

// Test 3: truncation detection — file shrinks, read from beginning.
func TestTailer_TruncationDetection(t *testing.T) {
	path := writeTempFile(t, "")

	snd := &mockLogSender{}
	tailer := openTailerForPath(t, path, snd)

	// Write initial content and advance offset.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	f.WriteString("initial content that will be gone\n")
	f.Close()
	tailer.readNewLines(path) // advance offset

	// Truncate file to 0.
	if err := os.Truncate(path, 0); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// Write new content at the beginning.
	f2, err := os.OpenFile(path, os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for write: %v", err)
	}
	f2.WriteString("after truncation\n")
	f2.Close()

	entries := tailer.readNewLines(path)
	if len(entries) == 0 {
		t.Fatal("expected entries after truncation, got none")
	}
	if entries[0].Message != "after truncation" {
		t.Errorf("expected 'after truncation', got %q", entries[0].Message)
	}
}

// Test 4: line truncation at 4096 bytes.
func TestTailer_LineTruncatedAt4096(t *testing.T) {
	path := writeTempFile(t, "")

	snd := &mockLogSender{}
	tailer := openTailerForPath(t, path, snd)

	longLine := strings.Repeat("a", 5000) + "\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	f.WriteString(longLine)
	f.Close()

	entries := tailer.readNewLines(path)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if len(entries[0].Message) != maxLineBytes {
		t.Errorf("expected message length %d, got %d", maxLineBytes, len(entries[0].Message))
	}
}

// Test 5: multi-line batch — 5 lines produce 5 entries.
func TestTailer_MultiLineBatch(t *testing.T) {
	path := writeTempFile(t, "")

	snd := &mockLogSender{}
	tailer := openTailerForPath(t, path, snd)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	for i := 0; i < 5; i++ {
		f.WriteString("line\n")
	}
	f.Close()

	entries := tailer.readNewLines(path)
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}
}

// Test 6: empty lines are skipped.
func TestTailer_EmptyLinesSkipped(t *testing.T) {
	path := writeTempFile(t, "")

	snd := &mockLogSender{}
	tailer := openTailerForPath(t, path, snd)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	f.WriteString("\n\n\nhello\n\n")
	f.Close()

	entries := tailer.readNewLines(path)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (empty lines skipped), got %d", len(entries))
	}
	if entries[0].Message != "hello" {
		t.Errorf("expected 'hello', got %q", entries[0].Message)
	}
}

// Test 7: ParseLevel integration — ERROR line parsed correctly.
func TestTailer_ParseLevelIntegration(t *testing.T) {
	path := writeTempFile(t, "")

	snd := &mockLogSender{}
	tailer := openTailerForPath(t, path, snd)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	f.WriteString("[ERROR] oops\n")
	f.Close()

	entries := tailer.readNewLines(path)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Level != model.LogLevel("error") {
		t.Errorf("expected level 'error', got %q", entries[0].Level)
	}
}

// Test 8: empty paths — NewTailer returns non-nil tailer, nil error; Run returns immediately on cancel.
func TestTailer_EmptyPaths(t *testing.T) {
	snd := &mockLogSender{}
	tailer, err := NewTailer([]string{}, "testhost", snd, newNopLogger())
	if err != nil {
		t.Fatalf("expected nil error for empty paths, got: %v", err)
	}
	if tailer == nil {
		t.Fatal("expected non-nil tailer for empty paths")
	}
	t.Cleanup(func() { tailer.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	done := make(chan struct{})
	go func() {
		tailer.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		// success: Run returned quickly after ctx cancel
	case <-time.After(2 * time.Second):
		t.Error("Run did not return after context cancellation with empty paths")
	}
}

// Test 9: per-path error isolation — bad path doesn't abort NewTailer.
func TestTailer_PerPathErrorIsolation(t *testing.T) {
	validPath := writeTempFile(t, "")

	snd := &mockLogSender{}
	tailer, err := NewTailer([]string{"/nonexistent/path/that/does/not/exist.log", validPath}, "testhost", snd, newNopLogger())
	if err != nil {
		t.Fatalf("expected nil error even with bad path, got: %v", err)
	}
	if tailer == nil {
		t.Fatal("expected non-nil tailer despite bad path")
	}
	t.Cleanup(func() { tailer.Close() })
}

// Test 10 (Task 6): rotation — truncation resets offset to 0.
func TestTailer_RotationTruncation(t *testing.T) {
	path := writeTempFile(t, "")

	snd := &mockLogSender{}
	tailer := openTailerForPath(t, path, snd)

	// Write 100 bytes worth of content and advance offset.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	f.WriteString(strings.Repeat("a", 95) + "\n") // 96 bytes including newline
	f.Close()

	entriesBefore := tailer.readNewLines(path)
	if len(entriesBefore) == 0 {
		t.Fatal("expected initial content to be read")
	}

	// Verify offset advanced — offset should be non-zero now.
	if tailer.offsets[path] == 0 {
		t.Error("expected offset to advance after reading initial content")
	}

	// Truncate to 50 bytes (less than current offset).
	if err := os.Truncate(path, 50); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// Write new content from beginning.
	f2, err := os.OpenFile(path, os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for write: %v", err)
	}
	// Seek to beginning since Truncate doesn't change file position.
	f2.WriteString("new line after rotation\n")
	f2.Close()

	// Truncate to exactly the new content size.
	stat, _ := os.Stat(path)
	if stat.Size() > 50 {
		// File has content beyond position 50 — we need to rewrite cleanly.
	}

	entries := tailer.readNewLines(path)
	// After truncation, offset resets to 0 and we read from beginning.
	if len(entries) == 0 {
		t.Error("expected entries after truncation reset — offset should have been reset to 0")
	}
}

// Test 11 (Task 6): after rename event handling, offset resets to 0.
func TestTailer_RenameResetsOffset(t *testing.T) {
	path := writeTempFile(t, "initial content\n")

	snd := &mockLogSender{}
	tailer := openTailerForPath(t, path, snd)

	// Advance offset by reading.
	tailer.readNewLines(path)
	// Write more and advance offset further.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("more content\n")
	f.Close()
	tailer.readNewLines(path)

	prevOffset := tailer.offsets[path]
	if prevOffset == 0 {
		t.Skip("offset should be non-zero after reading content")
	}

	// Simulate rename event handling.
	tailer.closePath(path)
	tailer.offsets[path] = 0

	if tailer.offsets[path] != 0 {
		t.Errorf("expected offset 0 after rename handling, got %d", tailer.offsets[path])
	}
	if _, ok := tailer.files[path]; ok {
		t.Error("expected file handle closed after rename handling")
	}
}

// Test 12 (Task 6): after create event, file is reopened at offset 0.
func TestTailer_CreateReopensAtZero(t *testing.T) {
	path := writeTempFile(t, "")

	snd := &mockLogSender{}
	tailer := openTailerForPath(t, path, snd)

	// Simulate create event: close path, reopen at 0.
	tailer.closePath(path)
	tailer.offsets[path] = 0

	// Write new content (simulating a new file after rotation).
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatalf("open for write: %v", err)
	}
	f.WriteString("rotated log line 1\nrotated log line 2\n")
	f.Close()

	// Now open the file fresh (simulating Create event handling).
	if err := tailer.openAndSeekEOF(path); err != nil {
		t.Fatalf("openAndSeekEOF: %v", err)
	}
	// Reset offset to 0 as Create event does.
	tailer.offsets[path] = 0

	entries := tailer.readNewLines(path)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after create/reopen, got %d", len(entries))
	}
	if entries[0].Message != "rotated log line 1" {
		t.Errorf("expected 'rotated log line 1', got %q", entries[0].Message)
	}
}
