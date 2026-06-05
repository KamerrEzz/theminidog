package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kamerrezz/theminidog/internal/server/storage"
)

// Op is a comparison operator for alert threshold rules.
type Op string

const (
	OpGT Op = ">"
	OpLT Op = "<"
)

// Rule defines a single threshold alert rule.
type Rule struct {
	Host      string        // "*" = all known hosts
	Name      string        // canonical metric name
	Op        Op            // ">" or "<"
	Threshold float64       // comparison value
	For       time.Duration // evaluation window; parsed from "5m" etc.
}

// AlertState represents the current state of an alert.
type AlertState string

const (
	StateFiring AlertState = "firing"
	StateOK     AlertState = "ok"
)

// Alert is a snapshot of one rule's current evaluation result.
type Alert struct {
	Rule      Rule       `json:"rule"`
	State     AlertState `json:"state"`
	UpdatedAt time.Time  `json:"updated_at"`
	Value     float64    `json:"value"`
}

// AlertReader is the read-only view of alert state for the dashboard and API.
type AlertReader interface {
	ActiveAlerts() []Alert
}

// MetricQuerier is the minimal storage interface alerting needs.
// Declared here (point of use) to avoid circular imports.
type MetricQuerier interface {
	Query(ctx context.Context, params storage.QueryParams) ([]storage.QueryPoint, error)
	Hosts(ctx context.Context, window time.Duration) ([]string, error)
}

// ruleJSON is the deserialization shape for Rule.
// Rule.For is a time.Duration but JSON carries it as a string ("5m"); we parse
// via time.ParseDuration inside ParseRules (ADR-1).
type ruleJSON struct {
	Host      string  `json:"host"`
	Name      string  `json:"name"`
	Op        string  `json:"op"`
	Threshold float64 `json:"threshold"`
	For       string  `json:"for"`
}

// ParseRules parses the ALERT_RULES JSON string into []Rule.
// An empty string disables alerting and returns nil, nil.
func ParseRules(raw string) ([]Rule, error) {
	if raw == "" {
		return nil, nil
	}
	var rjs []ruleJSON
	if err := json.Unmarshal([]byte(raw), &rjs); err != nil {
		return nil, fmt.Errorf("parse ALERT_RULES: %w", err)
	}
	if len(rjs) > 20 {
		return nil, fmt.Errorf("ALERT_RULES: maximum 20 rules, got %d", len(rjs))
	}
	rules := make([]Rule, 0, len(rjs))
	for i, rj := range rjs {
		if rj.Host == "" {
			return nil, fmt.Errorf("rule[%d]: host must not be empty", i)
		}
		if rj.Name == "" {
			return nil, fmt.Errorf("rule[%d]: name must not be empty", i)
		}
		if rj.Op != ">" && rj.Op != "<" {
			return nil, fmt.Errorf("rule[%d]: op must be \">\" or \"<\", got %q", i, rj.Op)
		}
		d, err := time.ParseDuration(rj.For)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("rule[%d]: for %q is not a valid positive duration", i, rj.For)
		}
		rules = append(rules, Rule{
			Host:      rj.Host,
			Name:      rj.Name,
			Op:        Op(rj.Op),
			Threshold: rj.Threshold,
			For:       d,
		})
	}
	return rules, nil
}

// ruleKey produces the map key for the alert state map.
// Format: "{host}/{name}/{op}/{threshold}"
func ruleKey(host, name string, op Op, threshold float64) string {
	return fmt.Sprintf("%s/%s/%s/%g", host, name, op, threshold)
}

// Option configures an Evaluator.
type Option func(*Evaluator)

// WithNotifiers attaches notifiers fired on FIRING/RESOLVED transitions.
// Dispatch is fire-and-forget — each Notifier runs in its own goroutine.
func WithNotifiers(n []Notifier) Option {
	return func(e *Evaluator) { e.notifiers = n }
}

// Evaluator runs threshold rules against metric data on a 30-second ticker.
type Evaluator struct {
	rules     []Rule
	repo      MetricQuerier
	mu        sync.RWMutex
	state     map[string]Alert
	log       *slog.Logger
	notifiers []Notifier
}

// NewEvaluator creates an Evaluator. log may be nil (falls back to slog.Default()).
// opts are applied after initialization; omitting them preserves backward compatibility.
func NewEvaluator(rules []Rule, repo MetricQuerier, log *slog.Logger, opts ...Option) *Evaluator {
	if log == nil {
		log = slog.Default()
	}
	e := &Evaluator{
		rules: rules,
		repo:  repo,
		state: make(map[string]Alert),
		log:   log,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// ActiveAlerts returns a snapshot of all current alert states.
// Safe to call on a nil *Evaluator — returns an empty (non-nil) slice.
func (e *Evaluator) ActiveAlerts() []Alert {
	if e == nil {
		return []Alert{}
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Alert, 0, len(e.state))
	for _, a := range e.state {
		out = append(out, a)
	}
	return out
}

// Run starts the 30-second non-overlapping evaluation ticker.
// Each tick wraps evaluate in a 20-second context timeout.
// Blocks until ctx is cancelled.
func (e *Evaluator) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			evalCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			e.evaluate(evalCtx)
			cancel()
		}
	}
}

// EvaluateForTest exposes evaluate for use in tests (black-box package tests
// cannot call the unexported method directly).
func (e *Evaluator) EvaluateForTest(ctx context.Context) {
	e.evaluate(ctx)
}

func (e *Evaluator) evaluate(ctx context.Context) {
	now := time.Now().UTC()
	for _, rule := range e.rules {
		hosts := e.hostsFor(ctx, rule)
		for _, host := range hosts {
			points, err := e.repo.Query(ctx, storage.QueryParams{
				Host:   host,
				Name:   rule.Name,
				From:   now.Add(-rule.For),
				To:     now,
				Bucket: "1m",
				Agg:    "avg",
			})
			if err != nil {
				e.log.WarnContext(ctx, "alert eval query failed",
					"rule", ruleKey(host, rule.Name, rule.Op, rule.Threshold),
					"err", err)
				continue
			}
			if len(points) == 0 {
				// No data — leave state unchanged (spec: no-data case).
				continue
			}

			// Average of bucket averages.
			sum := 0.0
			for _, p := range points {
				sum += p.Value
			}
			mean := sum / float64(len(points))

			firing := (rule.Op == OpGT && mean > rule.Threshold) ||
				(rule.Op == OpLT && mean < rule.Threshold)

			newState := StateOK
			if firing {
				newState = StateFiring
			}

			key := ruleKey(host, rule.Name, rule.Op, rule.Threshold)

			e.mu.Lock()
			prev, existed := e.state[key]
			e.state[key] = Alert{
				Rule: Rule{
					Host:      host,
					Name:      rule.Name,
					Op:        rule.Op,
					Threshold: rule.Threshold,
					For:       rule.For,
				},
				State:     newState,
				UpdatedAt: now,
				Value:     mean,
			}
			e.mu.Unlock()

			if !existed || prev.State != newState {
				if newState == StateFiring {
					e.log.ErrorContext(ctx, "alert firing",
						"host", host,
						"name", rule.Name,
						"op", string(rule.Op),
						"threshold", rule.Threshold,
						"value", mean)
					e.notifyAll(ctx, "firing", Rule{
						Host:      host,
						Name:      rule.Name,
						Op:        rule.Op,
						Threshold: rule.Threshold,
						For:       rule.For,
					}, mean, now)
				} else {
					e.log.InfoContext(ctx, "alert resolved",
						"host", host,
						"name", rule.Name,
						"value", mean)
					// Only notify resolved if we transitioned FROM firing (existed must be true).
					if existed && prev.State == StateFiring {
						e.notifyAll(ctx, "resolved", Rule{
							Host:      host,
							Name:      rule.Name,
							Op:        rule.Op,
							Threshold: rule.Threshold,
							For:       rule.For,
						}, mean, now)
					}
				}
			}
		}
	}
}

func (e *Evaluator) hostsFor(ctx context.Context, rule Rule) []string {
	if rule.Host != "*" {
		return []string{rule.Host}
	}
	hosts, err := e.repo.Hosts(ctx, 30*time.Minute)
	if err != nil {
		e.log.WarnContext(ctx, "alert eval hosts query failed", "err", err)
		return nil
	}
	return hosts
}

// notifyAll dispatches event to every notifier in its own goroutine (fire-and-forget).
// Errors are swallowed here; each Notifier logs its own failures.
// Uses context.WithoutCancel so the goroutine survives the 20s eval-context cancellation.
func (e *Evaluator) notifyAll(ctx context.Context, event string, rule Rule, value float64, at time.Time) {
	if len(e.notifiers) == 0 {
		return
	}
	ev := NotificationEvent{Event: event, Rule: rule, Value: value, FiredAt: at}
	for _, n := range e.notifiers {
		n := n
		go func() { _ = n.Notify(context.WithoutCancel(ctx), ev) }()
	}
}

// NotifyHostDown dispatches a synthetic host.down firing event to all configured
// notifiers. Nil-safe — safe to call on a nil *Evaluator.
func (e *Evaluator) NotifyHostDown(host string) {
	if e == nil {
		return
	}
	rule := Rule{Host: host, Name: "host.down", Op: OpGT, Threshold: 0}
	e.notifyAll(context.Background(), "firing", rule, 0, time.Now().UTC())
}
