package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"time"

	"github.com/kamerrezz/theminidog/internal/model"
)

// Sender ships a MetricBatch to the server.
type Sender interface {
	Send(ctx context.Context, batch model.MetricBatch) error
}

// permanentError signals the batch must be discarded (no retry).
type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// BackoffConfig holds retry parameters.
type BackoffConfig struct {
	Base   time.Duration // starting wait (default 1s)
	Max    time.Duration // cap (default 60s)
	Jitter float64       // fraction ±jitter (default 0.25)
}

// DefaultBackoff returns a BackoffConfig with production defaults.
func DefaultBackoff() BackoffConfig {
	return BackoffConfig{Base: time.Second, Max: 60 * time.Second, Jitter: 0.25}
}

// waitFor computes the backoff duration for a given attempt (0-indexed).
// attempt=0 → 0 (no wait before first try).
func waitFor(attempt int, cfg BackoffConfig, randFn func() float64) time.Duration {
	if attempt == 0 {
		return 0
	}
	exp := math.Min(float64(cfg.Max), float64(cfg.Base)*math.Pow(2, float64(attempt-1)))
	jitter := 1.0 + cfg.Jitter*(randFn()*2-1)
	d := time.Duration(exp * jitter)
	if d > cfg.Max {
		d = cfg.Max
	}
	return d
}

// HTTPSender sends metric batches over HTTP with exponential backoff.
type HTTPSender struct {
	url     string
	token   string // optional Bearer token; empty = no Authorization header
	client  *http.Client
	backoff BackoffConfig
	randFn  func() float64
	sleepFn func(ctx context.Context, d time.Duration) error
	log     *slog.Logger
}

// NewHTTPSender creates a sender with the given server URL.
// If log is nil, slog.Default() is used.
func NewHTTPSender(serverURL string, cfg BackoffConfig, log *slog.Logger) *HTTPSender {
	if log == nil {
		log = slog.Default()
	}
	return &HTTPSender{
		url:     serverURL,
		client:  &http.Client{Timeout: 10 * time.Second},
		backoff: cfg,
		randFn:  rand.Float64,
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

// withRandFn replaces the random function (for testing).
func (s *HTTPSender) withRandFn(fn func() float64) *HTTPSender {
	s.randFn = fn
	return s
}

// withSleepFn replaces the sleep function (for testing).
func (s *HTTPSender) withSleepFn(fn func(ctx context.Context, d time.Duration) error) *HTTPSender {
	s.sleepFn = fn
	return s
}

// WithToken sets the Bearer token sent on every request.
// Returns s for chaining: NewHTTPSender(...).WithToken(tok)
func (s *HTTPSender) WithToken(token string) *HTTPSender {
	s.token = token
	return s
}

// Send posts the batch to the server, retrying on transient errors.
func (s *HTTPSender) Send(ctx context.Context, batch model.MetricBatch) error {
	body, err := json.Marshal(batch)
	if err != nil {
		return permanentError{fmt.Errorf("marshal batch: %w", err)}
	}

	for attempt := 0; ; attempt++ {
		wait := waitFor(attempt, s.backoff, s.randFn)
		if err := s.sleepFn(ctx, wait); err != nil {
			return err // context cancelled
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
		if err != nil {
			return permanentError{fmt.Errorf("build request: %w", err)}
		}
		req.Header.Set("Content-Type", "application/json")
		if s.token != "" {
			req.Header.Set("Authorization", "Bearer "+s.token)
		}

		resp, err := s.client.Do(req)
		if err != nil {
			s.log.WarnContext(ctx, "send failed, retrying", "attempt", attempt, "err", err)
			continue
		}
		resp.Body.Close()

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return nil
		case resp.StatusCode >= 400 && resp.StatusCode < 500:
			s.log.ErrorContext(ctx, "server rejected batch, discarding", "status", resp.StatusCode)
			return permanentError{fmt.Errorf("server rejected batch: %d", resp.StatusCode)}
		default:
			s.log.WarnContext(ctx, "server error, retrying", "attempt", attempt, "status", resp.StatusCode)
		}
	}
}
