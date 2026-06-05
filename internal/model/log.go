package model

import (
	"fmt"
	"strings"
	"time"
)

// LogLevel is the severity of a log entry.
type LogLevel string

const (
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
	LevelDebug LogLevel = "debug"
)

var validLogLevels = map[LogLevel]struct{}{
	LevelInfo:  {},
	LevelWarn:  {},
	LevelError: {},
	LevelDebug: {},
}

// LogEntry represents a single log line read from a file.
type LogEntry struct {
	Time    time.Time `json:"time"`
	Host    string    `json:"host"`
	Path    string    `json:"path"`
	Level   LogLevel  `json:"level"`
	Message string    `json:"message"`
}

// LogBatch groups a set of LogEntry values collected from a single host.
type LogBatch struct {
	Host    string     `json:"host"`
	Entries []LogEntry `json:"entries"`
}

// Validate returns an error if the LogEntry is malformed.
func (e LogEntry) Validate() error {
	if strings.TrimSpace(e.Host) == "" {
		return fmt.Errorf("log entry host must not be empty")
	}
	if strings.TrimSpace(e.Message) == "" {
		return fmt.Errorf("log entry message must not be empty")
	}
	if e.Time.IsZero() {
		return fmt.Errorf("log entry time must not be zero")
	}
	if _, ok := validLogLevels[e.Level]; !ok {
		return fmt.Errorf("invalid log level %q: must be one of info, warn, error, debug", e.Level)
	}
	return nil
}

// Validate returns an error if the LogBatch is malformed.
func (b LogBatch) Validate() error {
	if strings.TrimSpace(b.Host) == "" {
		return fmt.Errorf("log batch host must not be empty")
	}
	if len(b.Entries) == 0 {
		return fmt.Errorf("log batch entries must not be empty")
	}
	if len(b.Entries) > 1000 {
		return fmt.Errorf("log batch exceeds maximum size of 1000 entries")
	}
	for i, e := range b.Entries {
		if err := e.Validate(); err != nil {
			return fmt.Errorf("entry[%d]: %w", i, err)
		}
	}
	return nil
}
