package logtail

import (
	"io"
	"log/slog"
)

// newNopLogger returns a slog.Logger that discards all output (for tests).
func newNopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
