# Under the Hood — How the Code Actually Works

This page walks through the most interesting technical problems in MiniObserv and shows exactly how the code solves them. Every snippet comes from the real source files — nothing is simplified for illustration.

The goal is not to explain every line. It is to show you the tricky parts and the thinking behind them, so you can take those ideas to your own projects.

---

## 1. How CPU usage is actually measured

Here is the thing most developers do not realize: **CPU percentage does not exist anywhere on the system.** There is no file you can open and read `cpu_usage = 73.2%` from.

What the Linux kernel exposes in `/proc/stat` is a set of **cumulative counters** — total time spent in each CPU mode since the machine booted. These are measured in "jiffies" (kernel time ticks):

```
cpu  123456 0 67890 9876543 1234 0 567 0 0 0
     ^user  ^nice ^sys    ^idle  ^iowait ...
```

To get a meaningful percentage you have to:

1. Read the counters at time T1
2. Wait a moment
3. Read the counters again at time T2
4. Subtract: `delta_busy = (user+sys+nice)_T2 - (user+sys+nice)_T1`
5. Calculate ratio: `busy / (busy + idle) * 100`

That is why the agent collects on a 10-second interval — you need two data points to compute a rate. There is no shortcut.

MiniObserv uses [gopsutil](https://github.com/shirou/gopsutil), a library that handles all the `/proc/stat` parsing. The collector just calls it:

```go
// Collect implements Collector. It returns one aggregate cpu.usage_pct metric
// with label core=total, plus one per logical core with label core=<index>.
func (c *CPUCollector) Collect(ctx context.Context) ([]model.Metric, error) {
    now := time.Now().UTC()

    // Aggregate (percpu=false)
    totals, err := c.statFn(ctx, 0, false)
    if err != nil {
        return nil, fmt.Errorf("cpu aggregate: %w", err)
    }

    // Per-core (percpu=true)
    perCore, err := c.statFn(ctx, 0, true)
    if err != nil {
        return nil, fmt.Errorf("cpu per-core: %w", err)
    }
    // ...build metric slice...
}
```

Notice that `statFn` is a field, not a direct call to `gopsutil`. That detail matters a lot — we will get back to it in section 3.

**Takeaway:** Whenever you need a rate — CPU%, bandwidth, requests/sec — you need two samples and a time window. Rates are always derivatives, never values you can directly observe.

---

## 2. Network metrics: same idea, different trap

`/proc/net/dev` exposes network interface counters the same way — cumulative total bytes received and sent since boot, not bytes per second:

```
Inter-|   Receive                                           |  Transmit
 face |bytes    packets errs drop ...                       | bytes    ...
    lo: 123456       89    0    0 ...                         123456 ...
  eth0: 987654321  7654    0    0 ...                         456789 ...
```

So the NetworkCollector applies the same delta pattern. But here there is an additional trap that catches people off guard.

**On the very first collection call, there is no previous sample to subtract from.** The code handles this explicitly:

```go
// Collect implements Collector. On the first call it seeds prev and returns
// nil (no metrics). On subsequent calls it computes byte deltas per interface.
func (c *NetworkCollector) Collect(ctx context.Context) ([]model.Metric, error) {
    // ...build current snapshot...

    // First call: seed prev and return empty slice.
    if c.prev == nil {
        c.prev = curr
        c.prevAt = now
        return nil, nil  // <-- no metrics emitted
    }

    // Compute deltas.
    for name, cs := range curr {
        ps, ok := c.prev[name]
        if !ok {
            continue // new interface appeared since last tick; skip
        }
        bytesIn := int64(cs.BytesRecv) - int64(ps.BytesRecv)
        bytesOut := int64(cs.BytesSent) - int64(ps.BytesSent)
        if bytesIn < 0 { bytesIn = 0 }   // guard against counter reset
        if bytesOut < 0 { bytesOut = 0 }
        // ...append metrics...
    }

    c.prev = curr
    c.prevAt = now
    return metrics, nil
}
```

Look at lines 82–86. After computing the delta, the code clamps negative values to zero. This handles **counter reset** — if the kernel resets a counter (reboot, driver reload), the subtraction produces a huge negative number. Clamping to zero means you lose one data point rather than emitting a wildly wrong spike. That is the correct tradeoff.

This also explains why network metrics show nothing on the very first collection tick after the agent starts. That behavior is intentional, not a bug.

**Takeaway:** Cumulative counters are everywhere — network bytes, disk I/O, HTTP request counts, error totals. Whenever you work with them, always store the previous value and always guard against negative deltas.

---

## 3. Making the collectors testable — the statFn pattern

Here is a problem: how do you write a unit test for a CPU collector?

If your collector calls `gopsutil.CPUPercent()` directly, your test will read the real CPU values of whatever machine runs the test. Those values are different on every machine, change every second, and tell you nothing about whether your code is correct. You would be testing the OS, not your logic.

The solution is **dependency injection at the simplest possible level** — a function field:

```go
// CPUCollector collects CPU usage metrics using an injectable statFn for
// testability. In production, statFn is cpu.PercentWithContext from gopsutil.
type CPUCollector struct {
    host   string
    statFn func(ctx context.Context, interval time.Duration, percpu bool) ([]float64, error)
}

// NewCPUCollector returns a CPUCollector wired to gopsutil for real OS data.
func NewCPUCollector(host string) *CPUCollector {
    return &CPUCollector{
        host:   host,
        statFn: gopsutilcpu.PercentWithContext,  // real implementation
    }
}
```

In production, `statFn` points to the real gopsutil function. In tests, you swap it for whatever you want:

```go
// makeCPUStatFn returns a statFn stub that delegates based on the percpu flag.
func makeCPUStatFn(
    totalResult []float64, totalErr error,
    perCoreResult []float64, perCoreErr error,
) func(ctx context.Context, interval time.Duration, percpu bool) ([]float64, error) {
    return func(_ context.Context, _ time.Duration, percpu bool) ([]float64, error) {
        if percpu {
            return perCoreResult, perCoreErr
        }
        return totalResult, totalErr
    }
}
```

And then a test becomes completely deterministic:

```go
func TestCPUCollector_Collect_ReturnsAggregateAndPerCore(t *testing.T) {
    // Stub: aggregate=40.0, per-core=[30.0, 50.0]
    statFn := makeCPUStatFn(
        []float64{40.0}, nil,
        []float64{30.0, 50.0}, nil,
    )
    c := &CPUCollector{host: "test-host", statFn: statFn}

    metrics, err := c.Collect(context.Background())
    // ...assert exactly 3 metrics, correct values, correct labels...
}
```

No real OS calls. No flaky behavior. The test runs in microseconds and produces the same result on every machine, forever.

Notice there is no framework here — no mock library, no DI container, nothing. Just a function field. This is Dependency Injection stripped to its essence.

**Takeaway:** Any time your code talks to the outside world — OS, network, database, clock, file system — inject it as a function or interface. Your tests become instant, deterministic, and reliable. You are testing YOUR logic, not the world around it.

---

## 4. Writing a test that fails first

Look at this test from the alert evaluator:

```go
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
}
```

Read this test from top to bottom. Notice that it tells a complete story before a single implementation line exists:

1. **Arrange**: set up a fake data source that returns CPU values of 95% — above the 90% threshold
2. **Act**: run one evaluation cycle
3. **Assert**: the evaluator must report exactly one alert in the `FIRING` state

When this test was written, the `Evaluator` did not exist yet. The test failed immediately — `RED`. That failure is the specification. It says exactly what the code must do.

Then the implementation was written to make it pass — `GREEN`.

Now look at the companion test that checks the opposite direction:

```go
func TestEvaluator_resolvesGT(t *testing.T) {
    callCount := 0
    q := &fakeQuerier{
        queryFn: func(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
            callCount++
            if callCount == 1 {
                return []storage.QueryPoint{{Value: 95.0}}, nil  // fires
            }
            return []storage.QueryPoint{{Value: 85.0}}, nil  // resolves
        },
    }
    // ...
    e.EvaluateForTest(context.Background())
    // assert StateFiring...
    e.EvaluateForTest(context.Background())
    // assert StateOK...
}
```

This test specifies the full state transition: `StateFiring` → `StateOK`. The implementation had to handle this case or the test would fail. The test is the requirements document — and unlike a real requirements document, it runs.

**Takeaway:** Tests are not about checking if code works. They are about specifying what the code SHOULD do before you write it. A test written after implementation is a sanity check. A test written before implementation is a design tool.

---

## 5. How the JWT works — no user table, no sessions

Traditional authentication requires a lot of infrastructure: a user table, session storage, cookie management, refresh token rotation. For a monitoring agent sending metrics to a server it trusts, all of that is overkill.

MiniObserv uses HS256 JWT — a much simpler model based on a **shared secret**.

At agent startup, a token is minted and signed with `AGENT_TOKEN`:

```go
// mintAgentToken creates a short-lived HS256 JWT signed with the given secret.
func mintAgentToken(secret string) (string, error) {
    claims := jwt.RegisteredClaims{
        Issuer:    "miniobserv-agent",
        ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
        IssuedAt:  jwt.NewNumericDate(time.Now()),
    }
    return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}
```

That token is attached to every HTTP request as a `Bearer` header. On the server side, the middleware verifies it:

```go
// JWTMiddleware validates Bearer JWT tokens using HS256.
// It enforces jwt.WithValidMethods([]string{"HS256"}) to block alg=none attacks.
func JWTMiddleware(secret []byte) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // ...extract token from Authorization header...
            _, err := jwt.ParseWithClaims(
                tokenStr,
                &jwt.RegisteredClaims{},
                func(t *jwt.Token) (any, error) { return secret, nil },
                jwt.WithValidMethods([]string{"HS256"}), // MANDATORY
            )
            if err != nil {
                writeError(w, http.StatusUnauthorized, "unauthorized")
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

The key line is `jwt.WithValidMethods([]string{"HS256"})`. This is not decoration — it blocks the `alg=none` attack, where a malicious token claims no signature is required. Without this guard, an attacker could craft a token that verifies against any key. The library requires you to explicitly opt in to which algorithms you trust.

The whole flow is: agent and server share the same secret → agent signs with it → server verifies the signature → if it matches, the request is authentic. No database lookup. No session state. Just math.

The tradeoff: if `AGENT_TOKEN` leaks, anyone can send metrics to your server. For an internal monitoring system running in a private network, this is the right tradeoff — simplicity wins.

**Takeaway:** For service-to-service auth (not user auth), a shared HMAC secret is often simpler and sufficient. No user table, no sessions, no cookies. Just two sides that know the same secret.

---

## 6. TimescaleDB — why not just PostgreSQL?

You could store metrics in a plain PostgreSQL table. It would work. But querying "give me average CPU in 5-minute buckets for the last hour" would look like this:

```sql
SELECT
    date_trunc('minute', time) - (EXTRACT(MINUTE FROM time)::int % 5) * INTERVAL '1 minute' AS bucket,
    AVG(value)
FROM metrics
WHERE host = 'web-01' AND name = 'cpu.usage_pct'
  AND time >= NOW() - INTERVAL '1 hour'
GROUP BY bucket
ORDER BY bucket DESC;
```

That is verbose, and more importantly, it is slow on large tables because PostgreSQL does not know this is time-series data — it cannot partition or index it efficiently by time ranges.

TimescaleDB adds one thing: `time_bucket()`. The migration that creates the metrics table does two critical things:

```sql
CREATE TABLE IF NOT EXISTS metrics (
    time   TIMESTAMPTZ      NOT NULL,
    host   TEXT             NOT NULL,
    name   TEXT             NOT NULL,
    value  DOUBLE PRECISION NOT NULL,
    labels JSONB
);

SELECT create_hypertable('metrics', 'time',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);
```

`create_hypertable` turns the table into a **hypertable** — under the hood, TimescaleDB splits it into chunks, one per day in this case. Each chunk is a separate physical table. Queries with a time range only touch the relevant chunks, not the entire dataset. Old chunks can be compressed or dropped automatically.

The query then becomes clean and fast:

```go
q := fmt.Sprintf(`
    SELECT time_bucket('%s', time) AS bucket,
           %s(value) AS value
    FROM metrics
    WHERE host = $1
      AND name = $2
      AND time >= $3
      AND time <= $4
    GROUP BY bucket
    ORDER BY bucket DESC`,
    bucketLiteral, aggFn,
)
```

Note: `bucketLiteral` and `aggFn` are never raw user input — they are resolved from an allowlist (`validBuckets` and `validAggs` maps) to prevent SQL injection. The bucket and aggregation function are looked up from safe values before being interpolated into the query.

Migration 003 adds compression and retention policies. Look at this line:

```sql
-- Step 2: Convert logs to a hypertable (logs was a plain BIGSERIAL table)
SELECT create_hypertable('logs', 'time', migrate_data => true, if_not_exists => true);

-- Step 6: Add retention policies (drop chunks older than threshold)
SELECT add_retention_policy('metrics', INTERVAL '30 days');
SELECT add_retention_policy('logs', INTERVAL '14 days');
```

The `logs` table had to be converted to a hypertable FIRST, THEN the retention policy was added. You cannot add a retention policy to a regular table — it only works on hypertables. That ordering in the migration is not arbitrary.

**Takeaway:** When your data is time-series — metrics, events, logs, audit trails — use a time-series database or extension. The query patterns are completely different from relational data, and the performance difference is dramatic at scale.

---

## 7. The alert state machine — why PENDING before FIRING

Look at the alert states in the evaluator:

```go
const (
    StateFiring AlertState = "firing"
    StateOK     AlertState = "ok"
)
```

There are only two persisted states. But the `for` field on a rule creates an implicit third state: **PENDING** — condition is true, but not for long enough yet.

Why does this matter? Consider a rule: "fire if CPU > 90% for 5 minutes". Without the `for` duration, a single spike to 95% for one second would trigger a notification. Your phone buzzes at 3am because a cron job ran. That is alert fatigue — the worst kind of noise in production monitoring.

The `for` duration solves this. The evaluator queries the average over the `for` window:

```go
points, err := e.repo.Query(ctx, storage.QueryParams{
    Host:   host,
    Name:   rule.Name,
    From:   now.Add(-rule.For),  // look back the full window
    To:     now,
    Bucket: "1m",
    Agg:    "avg",
})
// ...
// Average of bucket averages.
sum := 0.0
for _, p := range points {
    sum += p.Value
}
mean := sum / float64(len(points))

firing := (rule.Op == OpGT && mean > rule.Threshold) ||
          (rule.Op == OpLT && mean < rule.Threshold)
```

The condition only becomes `FIRING` when the average over the entire `for` window crosses the threshold. A one-second spike does not move the average enough to fire. A sustained problem does.

Then the state transition:

```go
e.mu.Lock()
prev, existed := e.state[key]
e.state[key] = Alert{...State: newState...}
e.mu.Unlock()

if !existed || prev.State != newState {
    if newState == StateFiring {
        e.notifyAll(ctx, "firing", ...)
    } else {
        // Only notify resolved if we transitioned FROM firing
        if existed && prev.State == StateFiring {
            e.notifyAll(ctx, "resolved", ...)
        }
    }
}
```

Notifications only fire on **transitions**, not on every tick. And the resolved notification only fires if the previous state was `FIRING` — if the alert was never firing, there is nothing to resolve. This prevents "resolved" spam for alerts that were never triggered.

**Takeaway:** State machines are everywhere in systems programming. When you have "if this condition persists for X time, do Y", you need to track state. Without it you get either false positives (too noisy) or missed alerts (too quiet).

---

## 8. Log tailing — how does it know when a file changes?

The naive approach to watching a log file is a polling loop:

```go
for {
    content := readFile("app.log")
    // diff against previous content
    time.Sleep(1 * time.Second)
}
```

This is slow, wastes CPU, misses changes that happen between polls, and does not scale to many files. There is a better way.

The `Tailer` uses [fsnotify](https://github.com/fsnotify/fsnotify), which wraps OS-level file system event APIs — `inotify` on Linux, `kqueue` on macOS, `ReadDirectoryChangesW` on Windows. The OS itself tells you when a file changes. No polling:

```go
// Run blocks until ctx is cancelled, processing fsnotify events.
func (t *Tailer) Run(ctx context.Context) {
    defer t.closeAll()
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
            // ...
        }
    }
}
```

The tailer sits in a `select` loop waiting for events from the OS. When a `Write` event arrives, it reads only the new bytes since the last offset — it never re-reads the whole file:

```go
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
            // ...
            return
        }
        t.offsets[path] = 0  // read from beginning of new file
        // ...
    }
}
```

Here is the tricky part: **log rotation**. When a logger rotates, it renames `app.log` to `app.log.1` and creates a new empty `app.log`. Your file descriptor still points to the old renamed file — you are reading from `app.log.1` now, not the active log.

The code handles this through the event stream. A `Rename` or `Remove` event closes the old handle and resets the offset to 0. A `Create` event (the new empty `app.log` appearing) reopens the file from the beginning. The `offsets[path] = 0` on `Create` ensures you read from the start of the new file rather than trying to seek to the old EOF position.

The initial `openAndSeekEOF` on startup does the opposite — it records the current file size as the starting offset, so pre-existing content is never sent. You only tail new lines written after the agent started.

**Takeaway:** For watching files, use OS events — not polling. And always handle rotation for log files. The file you opened at startup is not always the file you think you are reading from.

---

## What to take from all of this

None of these patterns are exotic or MiniObserv-specific. They appear in every serious Go codebase:

- **Delta calculations** for any rate metric
- **Injectable functions** for anything touching the OS or network
- **Tests written first** as executable specifications
- **Shared secret JWT** for service-to-service auth
- **Hypertables** for time-series data
- **State machines** for condition-based alerting
- **OS file events** for log tailing

If you internalized one thing from this page, make it this: the most important decision in any system is what NOT to build. Every section above is a case where a simpler approach was chosen deliberately — and the tradeoffs are explicit. That judgment is what separates a senior engineer from someone who just writes code.
