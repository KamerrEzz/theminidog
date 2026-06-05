package logtail

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kamerrezz/theminidog/internal/model"
)

const (
	maxLineBytes = 4096
	maxBatchSize = 1000
)

// Tailer watches log files and ships new lines as LogBatches.
type Tailer struct {
	paths   []string
	host    string
	sender  LogSender
	offsets map[string]int64
	files   map[string]*os.File
	watcher *fsnotify.Watcher
	log     *slog.Logger
}

// NewTailer creates a Tailer and opens each path, seeking to EOF.
// Per-path errors are logged and skipped — they do NOT abort construction.
func NewTailer(paths []string, host string, sender LogSender, log *slog.Logger) (*Tailer, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}
	t := &Tailer{
		paths:   paths,
		host:    host,
		sender:  sender,
		offsets: make(map[string]int64),
		files:   make(map[string]*os.File),
		watcher: watcher,
		log:     log,
	}
	for _, p := range paths {
		if err := t.openAndSeekEOF(p); err != nil {
			log.Warn("logtail: skipping path on open error", "path", p, "err", err)
			continue
		}
		if err := watcher.Add(p); err != nil {
			log.Warn("logtail: skipping path on watch error", "path", p, "err", err)
			t.closePath(p)
		}
	}
	return t, nil
}

// openAndSeekEOF opens a file and records its current size as the starting offset.
// CRITICAL: this ensures pre-existing content is never read.
func (t *Tailer) openAndSeekEOF(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	t.files[path] = f
	t.offsets[path] = info.Size() // seek to EOF — never read pre-existing content
	return nil
}

// Run blocks until ctx is cancelled, processing fsnotify events.
func (t *Tailer) Run(ctx context.Context) {
	defer t.closeAll()
	if len(t.paths) == 0 {
		<-ctx.Done()
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-t.watcher.Events:
			if !ok {
				return
			}
			t.handleEvent(ctx, event)
		case err, ok := <-t.watcher.Errors:
			if !ok {
				return
			}
			t.log.Warn("logtail: watcher error", "err", err)
		}
	}
}

func (t *Tailer) handleEvent(ctx context.Context, event fsnotify.Event) {
	path := event.Name
	switch {
	case event.Has(fsnotify.Write):
		entries := t.readNewLines(path)
		t.sendChunked(ctx, entries)
	case event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove):
		t.closePath(path)
		t.offsets[path] = 0
	case event.Has(fsnotify.Create):
		t.closePath(path)
		if err := t.openAndSeekEOF(path); err != nil {
			t.log.Warn("logtail: failed to reopen after create", "path", path, "err", err)
			return
		}
		// For Create after rotation: reset to 0 to read from beginning.
		t.offsets[path] = 0
		if err := t.watcher.Add(path); err != nil {
			t.log.Warn("logtail: failed to re-watch after create", "path", path, "err", err)
		}
	}
}

// readNewLines reads any new content since the last offset.
// Detects truncation (size < offset) and resets to beginning.
func (t *Tailer) readNewLines(path string) []model.LogEntry {
	f, ok := t.files[path]
	if !ok {
		var err error
		f, err = os.Open(path)
		if err != nil {
			t.log.Warn("logtail: cannot open file for reading", "path", path, "err", err)
			return nil
		}
		t.files[path] = f
	}

	info, err := f.Stat()
	if err != nil {
		t.log.Warn("logtail: stat failed", "path", path, "err", err)
		return nil
	}
	currentSize := info.Size()

	// Truncation detection: file shrank below our recorded offset.
	if currentSize < t.offsets[path] {
		t.offsets[path] = 0
	}

	if currentSize == t.offsets[path] {
		return nil // no new data
	}

	if _, err := f.Seek(t.offsets[path], io.SeekStart); err != nil {
		t.log.Warn("logtail: seek failed", "path", path, "err", err)
		return nil
	}

	// Use a buffer large enough for any incoming line.
	// We truncate long lines AFTER scanning — the scanner itself must not overflow.
	const scanBufSize = 64 * 1024 // 64 KiB — far exceeds any real log line
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, scanBufSize), scanBufSize)
	now := time.Now().UTC()
	var entries []model.LogEntry

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) > maxLineBytes {
			line = line[:maxLineBytes]
		}
		entries = append(entries, model.LogEntry{
			Time:    now,
			Host:    t.host,
			Path:    path,
			Level:   model.LogLevel(ParseLevel(line)),
			Message: line,
		})
	}

	// Advance offset to current file size.
	t.offsets[path] = currentSize
	return entries
}

// sendChunked splits entries into ≤1000-entry batches and sends each.
func (t *Tailer) sendChunked(ctx context.Context, entries []model.LogEntry) {
	if len(entries) == 0 {
		return
	}
	for i := 0; i < len(entries); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := model.LogBatch{Host: t.host, Entries: entries[i:end]}
		if err := t.sender.SendLogs(ctx, batch); err != nil {
			if ctx.Err() != nil {
				return
			}
			t.log.Error("logtail: send failed", "err", err)
		}
	}
}

// Close releases all file handles and the fsnotify watcher.
// Call this when the Tailer is no longer needed (e.g., in tests).
func (t *Tailer) Close() {
	t.closeAll()
}

func (t *Tailer) closePath(path string) {
	if f, ok := t.files[path]; ok {
		f.Close()
		delete(t.files, path)
	}
}

func (t *Tailer) closeAll() {
	for _, f := range t.files {
		f.Close()
	}
	t.watcher.Close()
}
