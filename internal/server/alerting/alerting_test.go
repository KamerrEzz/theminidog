package alerting_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kamerrezz/theminidog/internal/server/alerting"
	"github.com/kamerrezz/theminidog/internal/server/storage"
)

// fakeQuerier is a test double for alerting.MetricQuerier.
type fakeQuerier struct {
	queryFn func(ctx context.Context, params storage.QueryParams) ([]storage.QueryPoint, error)
	hostsFn func(ctx context.Context, window time.Duration) ([]string, error)
}

func (f *fakeQuerier) Query(ctx context.Context, params storage.QueryParams) ([]storage.QueryPoint, error) {
	if f.queryFn != nil {
		return f.queryFn(ctx, params)
	}
	return nil, nil
}

func (f *fakeQuerier) Hosts(ctx context.Context, window time.Duration) ([]string, error) {
	if f.hostsFn != nil {
		return f.hostsFn(ctx, window)
	}
	return nil, nil
}

// ---- ParseRules tests ----

func TestParseRules_valid(t *testing.T) {
	raw := `[
		{"host":"web-01","name":"cpu.usage_pct","op":">","threshold":90.0,"for":"5m"},
		{"host":"web-02","name":"mem.used_pct","op":"<","threshold":10.0,"for":"15m"}
	]`
	rules, err := alerting.ParseRules(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].For != 5*time.Minute {
		t.Fatalf("rule[0].For: expected 5m, got %v", rules[0].For)
	}
	if rules[1].For != 15*time.Minute {
		t.Fatalf("rule[1].For: expected 15m, got %v", rules[1].For)
	}
	if rules[0].Op != alerting.OpGT {
		t.Fatalf("rule[0].Op: expected >, got %v", rules[0].Op)
	}
	if rules[1].Op != alerting.OpLT {
		t.Fatalf("rule[1].Op: expected <, got %v", rules[1].Op)
	}
	if rules[0].Threshold != 90.0 {
		t.Fatalf("rule[0].Threshold: expected 90.0, got %v", rules[0].Threshold)
	}
}

func TestParseRules_empty(t *testing.T) {
	rules, err := alerting.ParseRules("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rules != nil {
		t.Fatalf("expected nil rules for empty input, got %v", rules)
	}
}

func TestParseRules_badOp(t *testing.T) {
	raw := `[{"host":"web-01","name":"cpu.usage_pct","op":">=","threshold":90.0,"for":"5m"}]`
	_, err := alerting.ParseRules(raw)
	if err == nil {
		t.Fatal("expected error for op >=, got nil")
	}
}

func TestParseRules_badDuration(t *testing.T) {
	raw := `[{"host":"web-01","name":"cpu.usage_pct","op":">","threshold":90.0,"for":"5minutes"}]`
	_, err := alerting.ParseRules(raw)
	if err == nil {
		t.Fatal("expected error for bad duration '5minutes', got nil")
	}
}

func TestParseRules_tooMany(t *testing.T) {
	// Build 21 valid rule objects
	buf := make([]byte, 0, 2048)
	buf = append(buf, '[')
	for i := 0; i < 21; i++ {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, []byte(`{"host":"web-01","name":"cpu.usage_pct","op":">","threshold":90.0,"for":"5m"}`)...)
	}
	buf = append(buf, ']')
	_, err := alerting.ParseRules(string(buf))
	if err == nil {
		t.Fatal("expected error for 21 rules, got nil")
	}
}

func TestParseRules_emptyHost(t *testing.T) {
	raw := `[{"host":"","name":"cpu.usage_pct","op":">","threshold":90.0,"for":"5m"}]`
	_, err := alerting.ParseRules(raw)
	if err == nil {
		t.Fatal("expected error for empty host, got nil")
	}
}

// ---- ActiveAlerts nil safety ----

func TestActiveAlerts_nil(t *testing.T) {
	var e *alerting.Evaluator
	alerts := e.ActiveAlerts()
	if alerts == nil {
		t.Fatal("expected non-nil []Alert{} from nil evaluator, got nil")
	}
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts from nil evaluator, got %d", len(alerts))
	}
}

// ---- Evaluator state machine tests ----

func TestEvaluator_firesOnGT(t *testing.T) {
	points := []storage.QueryPoint{
		{Time: time.Now().UTC(), Value: 95.0},
		{Time: time.Now().UTC().Add(-time.Minute), Value: 95.0},
	}
	q := &fakeQuerier{
		queryFn: func(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
			return points, nil
		},
	}
	rule := alerting.Rule{
		Host:      "web-01",
		Name:      "cpu.usage_pct",
		Op:        alerting.OpGT,
		Threshold: 90.0,
		For:       5 * time.Minute,
	}
	e := alerting.NewEvaluator([]alerting.Rule{rule}, q, nil)
	e.EvaluateForTest(context.Background())

	alerts := e.ActiveAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].State != alerting.StateFiring {
		t.Fatalf("expected StateFiring, got %v", alerts[0].State)
	}
	if alerts[0].Value != 95.0 {
		t.Fatalf("expected value 95.0, got %v", alerts[0].Value)
	}
}

func TestEvaluator_resolvesGT(t *testing.T) {
	// First call: avg=95, fires
	callCount := 0
	q := &fakeQuerier{
		queryFn: func(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
			callCount++
			if callCount == 1 {
				return []storage.QueryPoint{{Value: 95.0}}, nil
			}
			return []storage.QueryPoint{{Value: 85.0}}, nil
		},
	}
	rule := alerting.Rule{Host: "web-01", Name: "cpu.usage_pct", Op: alerting.OpGT, Threshold: 90.0, For: 5 * time.Minute}
	e := alerting.NewEvaluator([]alerting.Rule{rule}, q, nil)

	e.EvaluateForTest(context.Background())
	alerts := e.ActiveAlerts()
	if len(alerts) != 1 || alerts[0].State != alerting.StateFiring {
		t.Fatalf("expected StateFiring after first eval, got %v", alerts)
	}

	e.EvaluateForTest(context.Background())
	alerts = e.ActiveAlerts()
	if len(alerts) != 1 || alerts[0].State != alerting.StateOK {
		t.Fatalf("expected StateOK after second eval, got %v", alerts)
	}
}

func TestEvaluator_noData(t *testing.T) {
	// Start with a firing state, then return no data — state must remain firing.
	callCount := 0
	q := &fakeQuerier{
		queryFn: func(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
			callCount++
			if callCount == 1 {
				return []storage.QueryPoint{{Value: 95.0}}, nil
			}
			return nil, nil // no data
		},
	}
	rule := alerting.Rule{Host: "web-01", Name: "cpu.usage_pct", Op: alerting.OpGT, Threshold: 90.0, For: 5 * time.Minute}
	e := alerting.NewEvaluator([]alerting.Rule{rule}, q, nil)

	// First eval: fires
	e.EvaluateForTest(context.Background())
	// Second eval: no data → state unchanged (still firing)
	e.EvaluateForTest(context.Background())

	alerts := e.ActiveAlerts()
	if len(alerts) != 1 || alerts[0].State != alerting.StateFiring {
		t.Fatalf("expected StateFiring (no-data unchanged), got %v", alerts)
	}
}

func TestEvaluator_LT(t *testing.T) {
	q := &fakeQuerier{
		queryFn: func(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
			return []storage.QueryPoint{{Value: 10.0}}, nil
		},
	}
	rule := alerting.Rule{Host: "web-01", Name: "mem.used_pct", Op: alerting.OpLT, Threshold: 20.0, For: 5 * time.Minute}
	e := alerting.NewEvaluator([]alerting.Rule{rule}, q, nil)
	e.EvaluateForTest(context.Background())

	alerts := e.ActiveAlerts()
	if len(alerts) != 1 || alerts[0].State != alerting.StateFiring {
		t.Fatalf("expected StateFiring for LT rule, got %v", alerts)
	}
}

func TestEvaluator_wildcard(t *testing.T) {
	hostsQueried := make([]string, 0)
	var mu sync.Mutex

	q := &fakeQuerier{
		hostsFn: func(_ context.Context, _ time.Duration) ([]string, error) {
			return []string{"host-a", "host-b"}, nil
		},
		queryFn: func(_ context.Context, params storage.QueryParams) ([]storage.QueryPoint, error) {
			mu.Lock()
			hostsQueried = append(hostsQueried, params.Host)
			mu.Unlock()
			return []storage.QueryPoint{{Value: 95.0}}, nil
		},
	}
	rule := alerting.Rule{Host: "*", Name: "cpu.usage_pct", Op: alerting.OpGT, Threshold: 90.0, For: 5 * time.Minute}
	e := alerting.NewEvaluator([]alerting.Rule{rule}, q, nil)
	e.EvaluateForTest(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(hostsQueried) != 2 {
		t.Fatalf("expected 2 host queries (host-a, host-b), got %v", hostsQueried)
	}
	hasA, hasB := false, false
	for _, h := range hostsQueried {
		if h == "host-a" {
			hasA = true
		}
		if h == "host-b" {
			hasB = true
		}
	}
	if !hasA || !hasB {
		t.Fatalf("expected both host-a and host-b queried, got %v", hostsQueried)
	}
}

func TestEvaluator_race(t *testing.T) {
	q := &fakeQuerier{
		queryFn: func(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
			return []storage.QueryPoint{{Value: 95.0}}, nil
		},
	}
	rule := alerting.Rule{Host: "web-01", Name: "cpu.usage_pct", Op: alerting.OpGT, Threshold: 90.0, For: 5 * time.Minute}
	e := alerting.NewEvaluator([]alerting.Rule{rule}, q, nil)

	var wg sync.WaitGroup
	const goroutines = 10

	// Writer goroutines: call evaluate
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.EvaluateForTest(context.Background())
		}()
	}

	// Reader goroutines: call ActiveAlerts concurrently
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = e.ActiveAlerts()
		}()
	}

	wg.Wait()
}

// ---- WithNotifiers integration tests ----

// spyNotifier is a test double that captures all Notify calls.
type spyNotifier struct {
	mu     sync.Mutex
	events []alerting.NotificationEvent
}

func (s *spyNotifier) Notify(_ context.Context, ev alerting.NotificationEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
	return nil
}

func (s *spyNotifier) Events() []alerting.NotificationEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]alerting.NotificationEvent, len(s.events))
	copy(out, s.events)
	return out
}

// driveToFiring runs two evaluate cycles on the given evaluator:
// first with 'firingValue' data to push state to FIRING.
// Returns after the second evaluate call.
func driveToFiring(e *alerting.Evaluator, q *fakeQuerier, firingValue float64) {
	q.queryFn = func(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
		return []storage.QueryPoint{{Value: firingValue}}, nil
	}
	e.EvaluateForTest(context.Background())
}

func TestEvaluator_WithNotifiers_FiringTransition(t *testing.T) {
	rule := alerting.Rule{Host: "web-01", Name: "cpu.usage_pct", Op: alerting.OpGT, Threshold: 90.0, For: 5 * time.Minute}
	spy := &spyNotifier{}
	q := &fakeQuerier{}

	e := alerting.NewEvaluator([]alerting.Rule{rule}, q, nil, alerting.WithNotifiers([]alerting.Notifier{spy}))

	// Drive to FIRING (value > 90 threshold)
	driveToFiring(e, q, 95.0)

	// Allow goroutines to complete
	time.Sleep(50 * time.Millisecond)

	events := spy.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 notification event, got %d", len(events))
	}
	if events[0].Event != "firing" {
		t.Errorf("expected event='firing', got %q", events[0].Event)
	}
	if events[0].Rule.Name != rule.Name {
		t.Errorf("expected rule.Name=%q, got %q", rule.Name, events[0].Rule.Name)
	}
}

func TestEvaluator_WithNotifiers_ResolvedTransition(t *testing.T) {
	rule := alerting.Rule{Host: "web-01", Name: "cpu.usage_pct", Op: alerting.OpGT, Threshold: 90.0, For: 5 * time.Minute}
	spy := &spyNotifier{}
	callCount := 0
	q := &fakeQuerier{
		queryFn: func(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
			callCount++
			if callCount == 1 {
				return []storage.QueryPoint{{Value: 95.0}}, nil // FIRING
			}
			return []storage.QueryPoint{{Value: 80.0}}, nil // RESOLVED
		},
	}

	e := alerting.NewEvaluator([]alerting.Rule{rule}, q, nil, alerting.WithNotifiers([]alerting.Notifier{spy}))

	e.EvaluateForTest(context.Background())
	time.Sleep(50 * time.Millisecond)
	e.EvaluateForTest(context.Background())
	time.Sleep(50 * time.Millisecond)

	events := spy.Events()
	if len(events) != 2 {
		t.Fatalf("expected 2 notification events (firing + resolved), got %d: %v", len(events), events)
	}
	if events[0].Event != "firing" {
		t.Errorf("expected first event='firing', got %q", events[0].Event)
	}
	if events[1].Event != "resolved" {
		t.Errorf("expected second event='resolved', got %q", events[1].Event)
	}
}

func TestEvaluator_WithNotifiers_PendingNoNotify(t *testing.T) {
	rule := alerting.Rule{Host: "web-01", Name: "cpu.usage_pct", Op: alerting.OpGT, Threshold: 90.0, For: 5 * time.Minute}
	spy := &spyNotifier{}
	q := &fakeQuerier{
		queryFn: func(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
			// Value below threshold — stays OK/never fires
			return []storage.QueryPoint{{Value: 50.0}}, nil
		},
	}

	e := alerting.NewEvaluator([]alerting.Rule{rule}, q, nil, alerting.WithNotifiers([]alerting.Notifier{spy}))
	e.EvaluateForTest(context.Background())
	time.Sleep(50 * time.Millisecond)

	events := spy.Events()
	if len(events) != 0 {
		t.Errorf("expected 0 notify calls for non-firing state, got %d: %v", len(events), events)
	}
}

func TestEvaluator_NoNotifiers_Unchanged(t *testing.T) {
	// Zero-notifiers path: existing behavior must be preserved with no side effects.
	rule := alerting.Rule{Host: "web-01", Name: "cpu.usage_pct", Op: alerting.OpGT, Threshold: 90.0, For: 5 * time.Minute}
	q := &fakeQuerier{
		queryFn: func(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
			return []storage.QueryPoint{{Value: 95.0}}, nil
		},
	}

	// No WithNotifiers — must compile and behave as before
	e := alerting.NewEvaluator([]alerting.Rule{rule}, q, nil)
	e.EvaluateForTest(context.Background())

	alerts := e.ActiveAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].State != alerting.StateFiring {
		t.Fatalf("expected StateFiring, got %v", alerts[0].State)
	}
}
