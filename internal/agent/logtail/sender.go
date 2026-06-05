package logtail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	isender "github.com/kamerrezz/theminidog/internal/agent/sender"
	"github.com/kamerrezz/theminidog/internal/model"
)

// LogSender ships a LogBatch to the server.
type LogSender interface {
	SendLogs(ctx context.Context, batch model.LogBatch) error
}

// HTTPLogSender POSTs log batches with exponential backoff.
// Independent of the metrics HTTPSender — different endpoint, different payload.
type HTTPLogSender struct {
	url     string
	token   string
	client  *http.Client
	backoff isender.BackoffConfig
	randFn  func() float64
	sleepFn func(ctx context.Context, d time.Duration) error
	log     *slog.Logger
}

// NewHTTPLogSender creates an HTTPLogSender ready to POST log batches.
func NewHTTPLogSender(serverURL, token string, cfg isender.BackoffConfig, log *slog.Logger) *HTTPLogSender {
	return &HTTPLogSender{
		url:     serverURL,
		token:   token,
		client:  &http.Client{Timeout: 10 * time.Second},
		backoff: cfg,
		randFn:  func() float64 { return 0.5 },
		sleepFn: func(ctx context.Context, d time.Duration) error {
			if d <= 0 {
				return nil
			}
			select {
			case <-time.After(d):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		log: log,
	}
}

// SendLogs marshals the batch and POSTs it to the server with exponential backoff.
func (s *HTTPLogSender) SendLogs(ctx context.Context, batch model.LogBatch) error {
	body, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("marshal log batch: %w", err)
	}
	for attempt := 0; ; attempt++ {
		wait := logWaitFor(attempt, s.backoff, s.randFn)
		if err := s.sleepFn(ctx, wait); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if s.token != "" {
			req.Header.Set("Authorization", "Bearer "+s.token)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			s.log.WarnContext(ctx, "log send failed, retrying", "attempt", attempt, "err", err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			s.log.ErrorContext(ctx, "server rejected log batch, discarding", "status", resp.StatusCode)
			return fmt.Errorf("server rejected log batch: %d", resp.StatusCode)
		}
		s.log.WarnContext(ctx, "server error on log send, retrying", "attempt", attempt, "status", resp.StatusCode)
	}
}

// logWaitFor computes the backoff duration for a given attempt (0-indexed).
// attempt=0 → 0 (no wait before first try).
func logWaitFor(attempt int, cfg isender.BackoffConfig, randFn func() float64) time.Duration {
	if attempt == 0 {
		return 0
	}
	exp := math.Min(float64(cfg.Max), float64(cfg.Base)*math.Pow(2, float64(attempt)))
	jitter := 1.0 + cfg.Jitter*(randFn()*2-1)
	d := time.Duration(exp * jitter)
	if d > cfg.Max {
		d = cfg.Max
	}
	return d
}
