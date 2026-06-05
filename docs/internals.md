# MiniObserv Internals Guide

This guide teaches you how MiniObserv is actually built. It assumes you know Go and observability basics. What it gives you is the code structure, design patterns, and idioms you need on your first day contributing to or extending this codebase.

---

## 1. Codebase Map

```
theminidog/
├── cmd/
│   ├── agent/
│   │   └── main.go              Composition root for the agent binary. Wires config → collectors → sender → agent.
│   ├── server/
│   │   └── main.go              Composition root for the server binary. Wires config → DB pool → migrations → HTTP.
│   └── stubserver/
│       └── main.go              Minimal fake server for local agent testing without a real DB.
│
├── internal/
│   ├── model/
│   │   ├── metric.go            Metric + MetricBatch structs, 9-name allowlist, Validate(). Central domain type.
│   │   ├── metric_test.go       Unit tests for Metric.Validate() and MetricBatch.Validate().
│   │   ├── log.go               LogEntry + LogBatch structs, level allowlist, Validate().
│   │   └── log_test.go          Unit tests for log model validation.
│   │
│   ├── config/
│   │   ├── agent.go             LoadAgent() — reads all agent env vars, validates, returns AgentConfig.
│   │   ├── agent_test.go        Unit tests for agent config loading and parseDuration bounds.
│   │   ├── server.go            LoadServerConfig() — reads all server env vars, returns ServerConfig.
│   │   └── server_test.go       Unit tests for server config loading.
│   │
│   ├── agent/
│   │   ├── agent.go             Agent struct, Run(), collectLoop/senderLoop goroutines, channel.
│   │   ├── agent_test.go        Unit tests for agent lifecycle, channel buffering, drop behavior.
│   │   │
│   │   ├── collector/
│   │   │   ├── collector.go     Collector interface + Registry.CollectAll(). Add new collectors here.
│   │   │   ├── collector_test.go Tests for Registry (error isolation, multi-collector behavior).
│   │   │   ├── cpu.go           CPUCollector — statFn injection pattern, aggregate + per-core metrics.
│   │   │   ├── cpu_test.go      CPU collector tests via injected statFn (no real OS calls).
│   │   │   ├── memory.go        MemoryCollector — statFn injection, three mem.* metrics.
│   │   │   ├── memory_test.go   Memory collector tests.
│   │   │   ├── disk.go          DiskCollector — partFn + usageFn injection, per-mount metrics.
│   │   │   ├── disk_test.go     Disk collector tests.
│   │   │   ├── network.go       NetworkCollector — delta semantics, ioFn injection, first-call seed.
│   │   │   └── network_test.go  Network collector tests (seed call, delta, counter wrap clamping).
│   │   │
│   │   ├── sender/
│   │   │   ├── sender.go        HTTPSender, BackoffConfig, waitFor, WithToken, permanentError.
│   │   │   └── sender_test.go   Sender tests: retry on 503, no-retry on 4xx, ctx cancel, backoff math.
│   │   │
│   │   └── logtail/
│   │       ├── parser.go        ParseLevel() — regex-based log level extraction from raw log lines.
│   │       └── parser_test.go   Parser tests: JSON logs, bracketed, prefixed, first-word heuristics.
│   │
│   └── server/
│       ├── server.go            Server struct wrapping http.Server + pgxpool. Start/Shutdown lifecycle.
│       │
│       ├── api/
│       │   ├── router.go        NewRouter() — chi wiring, global middleware, JWT group, route registration.
│       │   ├── middleware.go     JWTMiddleware() — Bearer token parsing, WithValidMethods HS256 guard.
│       │   ├── middleware_test.go Handler-level tests for JWT middleware via httptest.
│       │   ├── metrics.go        HandleIngest + HandleQuery — the two metric endpoints.
│       │   ├── metrics_test.go   Handler tests using fakeRepo, httptest.NewRecorder.
│       │   ├── health.go         HandleHealthz (liveness) + HandleReadyz (readiness with DB ping).
│       │   ├── health_test.go    Handler tests for health endpoints.
│       │   ├── errors.go         writeError() helper — JSON error envelope, used by all handlers.
│       │   └── testhelpers_test.go  fakeRepo, mustDecode, errBody — shared test utilities.
│       │
│       └── storage/
│           ├── metrics.go        MetricRepository interface + pgxMetricRepository (pgx batch insert, query).
│           ├── metrics_test.go   Pure unit tests for QueryParams.Validate().
│           └── metrics_integration_test.go  Integration guard (//go:build integration).
│
├── migrations/                  SQL migration files managed by golang-migrate.
├── example/                     Standalone usage examples.
└── tools.go                     go:generate tool dependencies (blank imports).
```

**Navigation heuristic**: changing a collector? Start in `internal/agent/collector/`. Adding a new endpoint? Start in `internal/server/api/metrics.go` then `storage/metrics.go`. Changing how the agent ships data? `internal/agent/sender/sender.go`. Environment variables? `internal/config/`.

---

## 2. The Layering Rule

The import graph flows one direction only:

```
cmd/*                  (composition root — imports everything, instantiates nothing reusable)
  └─ internal/*/
       ├─ model/       (zero project imports — only stdlib)
       ├─ config/      (stdlib only — os, time, net/url, strings)
       ├─ agent/       (imports model, config; uses gopsutil)
       └─ server/      (imports model, config; uses pgx, chi, jwt)
```

`agent` and `server` are siblings. Neither imports the other. If you add `import "github.com/kamerrezz/theminidog/internal/agent"` inside any server package, the Go compiler will refuse to build with:

```
import cycle not allowed
package github.com/kamerrezz/theminidog/internal/server/api
    imports github.com/kamerrezz/theminidog/internal/agent
    imports github.com/kamerrezz/theminidog/internal/agent/sender
    (already seen)
```

The `model` package sits at the bottom of the graph specifically so both sides can use `model.Metric` and `model.MetricBatch` without a cycle. It imports nothing from this project.

**How to verify the graph is intact:**

```bash
go list -f '{{ .ImportPath }}: {{ join .Imports " " }}' ./...
```

Check the output for any line where a `server/...` package lists an `agent/...` import or vice versa. There should be none.

The `agent.go` file demonstrates the pattern used when two packages would otherwise couple: local interface definitions.

```go
// agent.go — agent does NOT import sender package directly
type senderIface interface {
    Send(ctx context.Context, batch model.MetricBatch) error
}
```

`*sender.HTTPSender` satisfies this interface implicitly. The `cmd/agent/main.go` composition root imports both `agent` and `sender` and wires them together. Neither package imports the other.

---

## 3. The Narrow Model

`internal/model/metric.go` in full:

```go
var validMetricNames = map[string]struct{}{
    "cpu.usage_pct":    {},
    "mem.used_pct":     {},
    "mem.used_bytes":   {},
    "mem.total_bytes":  {},
    "disk.used_pct":    {},
    "disk.used_bytes":  {},
    "disk.total_bytes": {},
    "net.bytes_in":     {},
    "net.bytes_out":    {},
}

type Metric struct {
    Time   time.Time         `json:"time"`
    Host   string            `json:"host"`
    Name   string            `json:"name"`
    Value  float64           `json:"value"`
    Labels map[string]string `json:"labels,omitempty"`
}

type MetricBatch struct {
    Host    string   `json:"host"`
    Metrics []Metric `json:"metrics"`
}

func (m Metric) Validate() error {
    if strings.TrimSpace(m.Host) == "" {
        return fmt.Errorf("metric host must not be empty")
    }
    if _, ok := validMetricNames[m.Name]; !ok {
        return fmt.Errorf("unknown metric name %q", m.Name)
    }
    if m.Time.IsZero() {
        return fmt.Errorf("metric time must not be zero")
    }
    if math.IsNaN(m.Value) || math.IsInf(m.Value, 0) {
        return fmt.Errorf("metric value must be finite")
    }
    for k, v := range m.Labels {
        if k == "" || v == "" {
            return fmt.Errorf("metric label key and value must not be empty")
        }
    }
    return nil
}
```

**Why the 9-name allowlist?** Without it, a collector typo like `"cpu_usage_pct"` (underscore instead of dot) would produce a metric that passes validation, gets stored, and then never shows up in queries that ask for `"cpu.usage_pct"`. The bug would be silent and confusing. The allowlist makes it loud: `Validate()` returns an error immediately, the batch is rejected, and the log shows exactly which name was wrong.

**Why `Labels map[string]string`?** Labels are stored as JSONB in PostgreSQL. The alternative — a separate `metric_labels` join table — requires a JOIN on every query and schema changes every time you add a new label key. JSONB gives you arbitrary key-value pairs in a single column with no schema changes and reasonable query performance for the access patterns used here.

**Why is `Validate()` on the model, not the handler?** Because validation is a property of the data contract, not of the transport. The same `Validate()` is called in two places:

- **Agent sender** (before pushing): `batch.Validate()` runs in `HandleIngest` on the server side, but the agent also calls it implicitly by construction — collectors return only canonical names, and the batch is built from those.
- **Server handler** (after receiving): `HandleIngest` calls `batch.Validate()` at line 27 before touching storage.

If validation lived only in the handler, a future internal caller (a test, a tool, a batch importer) could bypass it by calling storage directly. Placing `Validate()` on the type makes it impossible to ignore.

---

## 4. The Collector Pattern: statFn Injection

Every collector in this project follows the same structure. Here is `CPUCollector` as the reference:

```go
type CPUCollector struct {
    host   string
    statFn func(ctx context.Context, interval time.Duration, percpu bool) ([]float64, error)
}

func NewCPUCollector(host string) *CPUCollector {
    return &CPUCollector{
        host:   host,
        statFn: gopsutilcpu.PercentWithContext, // real OS call in production
    }
}
```

The `statFn` field holds the OS call. In production it is `gopsutilcpu.PercentWithContext`. In tests you replace it with a function literal:

```go
// From cpu_test.go
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

func TestCPUCollector_Collect_ReturnsAggregateAndPerCore(t *testing.T) {
    statFn := makeCPUStatFn(
        []float64{40.0}, nil,
        []float64{30.0, 50.0}, nil,
    )
    c := &CPUCollector{host: "test-host", statFn: statFn}

    metrics, err := c.Collect(context.Background())
    // ... assert metric names, labels, values
}
```

**Why not mock an interface?** Because the thing worth testing is not "did we call gopsutil" — it is the transformation logic: the correct metric name `"cpu.usage_pct"`, the correct label `core=total` for the aggregate, the correct label `core=0`, `core=1` etc. for per-core metrics. The `statFn` injection lets you control the raw OS output and verify what the collector does with it, with no mock library and no build tags.

The same pattern appears in all four collectors: `MemoryCollector.statFn`, `DiskCollector.partFn`+`usageFn`, and `NetworkCollector.ioFn`.

**How to add a new collector:**

1. Create `internal/agent/collector/yourname.go`. Define a struct with `host string` and a `statFn` field matching the gopsutil function signature you need.
2. Implement `Name() string` (short lowercase name, e.g. `"gpu"`) and `Collect(ctx context.Context) ([]model.Metric, error)`.
3. Add the new metric names to `validMetricNames` in `internal/model/metric.go`. Keep both allowlists in sync — `storage/metrics.go` has its own copy that mirrors the model's.
4. Wire it in `cmd/agent/main.go`:
   ```go
   reg := collector.NewRegistry(
       collector.NewCPUCollector(cfg.AgentHost),
       collector.NewMemoryCollector(cfg.AgentHost),
       collector.NewDiskCollector(cfg.AgentHost, cfg.DiskMounts),
       collector.NewNetworkCollector(cfg.AgentHost, cfg.NetIfaces),
       collector.NewYourCollector(cfg.AgentHost), // add here
   )
   ```
5. Write a test using an injected `statFn` that returns known data. Verify name, host, labels, and value without touching the OS.

---

## 5. The Two-Goroutine Agent

`internal/agent/agent.go` runs exactly two goroutines that communicate through a buffered channel:

```go
func (a *Agent) Run(ctx context.Context) {
    var wg sync.WaitGroup
    wg.Add(2)

    go func() {
        defer wg.Done()
        defer close(a.batches)   // signals senderLoop to drain and exit
        a.collectLoop(ctx)
    }()

    go func() {
        defer wg.Done()
        a.senderLoop(ctx)
    }()

    wg.Wait()
}
```

```
collectLoop          │  channel (buf=10)   │  senderLoop
                     │                     │
ticker.C ──────────► │ ──────────────────► │ ──────────► sender.Send(...)
non-blocking select  │ full? drop newest   │   blocks until send completes
```

**collectLoop** fires on each ticker tick, runs all collectors via `registry.CollectAll`, assembles a `MetricBatch`, and tries a non-blocking send:

```go
select {
case a.batches <- batch:
default:
    a.log.WarnContext(ctx, "batch channel full, dropping")
}
```

**senderLoop** ranges over the channel, blocking until a batch arrives, then calls `sender.Send`.

```go
func (a *Agent) senderLoop(ctx context.Context) {
    for batch := range a.batches {
        if err := a.sender.Send(ctx, batch); err != nil {
            // ...
        }
    }
}
```

**Channel buffer of 10.** If the server is slow (or unreachable and the sender is in backoff), the collect loop can keep running and queue up to 10 batches before dropping. This prevents a slow network from blocking collection.

**Drop newest, not oldest.** The non-blocking `select` with `default` drops the batch that just arrived when the channel is full. At first this seems backwards — shouldn't you keep the newest data? In practice, when the server is down, the oldest queued data marks the start of the outage. That is the most interesting data for diagnosis. Once 10 batches are queued, you already have the outage onset; dropping additional new points is acceptable.

**Graceful shutdown.** When `ctx` is cancelled, `collectLoop` returns because its `select` catches `ctx.Done()`. The deferred `close(a.batches)` then fires. Because `senderLoop` uses `for range` over the channel, `close` causes the range to drain any remaining queued batches before exiting. Both goroutines complete, `wg.Wait()` returns, and `Run` returns.

---

## 6. Exponential Backoff with Jitter

`internal/agent/sender/sender.go` contains `waitFor`:

```go
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
```

With `DefaultBackoff()` (Base=1s, Max=60s, Jitter=0.25):

| Attempt | Base wait | Jitter range    |
|---------|-----------|-----------------|
| 0       | 0         | no wait         |
| 1       | 1s        | [0.75s, 1.25s]  |
| 2       | 2s        | [1.5s, 2.5s]    |
| 3       | 4s        | [3s, 5s]        |
| 4       | 8s        | [6s, 10s]       |
| 5       | 16s       | [12s, 20s]      |
| 6       | 32s       | [24s, 40s]      |
| 7+      | capped 60s| [45s, 75s]      |

**Why jitter?** Imagine 100 agent processes that all lose connectivity at the same second (network blip, deploy, restart). Without jitter, they all compute the same wait duration and all retry at the same moment — a thundering herd that hammers the server right when it is recovering. Jitter spreads those 100 retries across a 50% window around the base duration so the load is distributed.

**Why ±25%?** It is the sweet spot between spread (higher jitter = more distributed) and predictability (lower jitter = easier to reason about SLOs). The `sender_test.go` verifies the math with deterministic `randFn` values:

```go
// From sender_test.go
func TestWaitFor_table(t *testing.T) {
    cfg := DefaultBackoff()
    tests := []struct {
        attempt int
        wantMin time.Duration
        wantMax time.Duration
    }{
        {0, 0, 0},
        {1, 750 * time.Millisecond, 1250 * time.Millisecond},
        {2, 1500 * time.Millisecond, 2500 * time.Millisecond},
        {7, 45 * time.Second, 75 * time.Second},
    }
    // ...
}
```

The test passes `constRand` (always returns 0.5) and full-range `randFn` values of 0.0 and 1.0 to verify the bounds. The key design point: `withSleepFn(noopSleep)` replaces the real `time.After` with a no-op so the tests run in microseconds.

**Permanent errors skip the retry loop.** A 4xx response wraps the error in `permanentError`:

```go
case resp.StatusCode >= 400 && resp.StatusCode < 500:
    return permanentError{fmt.Errorf("server rejected batch: %d", resp.StatusCode)}
```

The Send loop does not retry permanentErrors — the batch is malformed and retrying would produce the same result. 5xx responses and network errors are transient and retry indefinitely.

---

## 7. JWT Middleware Deep Dive

`internal/server/api/middleware.go`:

```go
func JWTMiddleware(secret []byte) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
                writeError(w, http.StatusUnauthorized, "unauthorized")
                return
            }
            tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
            if tokenStr == "" {
                writeError(w, http.StatusUnauthorized, "unauthorized")
                return
            }
            _, err := jwt.ParseWithClaims(
                tokenStr,
                &jwt.RegisteredClaims{},
                func(t *jwt.Token) (any, error) { return secret, nil },
                jwt.WithValidMethods([]string{"HS256"}), // MANDATORY — blocks alg=none and RS256
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

**Line by line:**

`strings.HasPrefix(authHeader, "Bearer ")` is a fast-path check before any JWT parsing. If the header is missing or malformed, we reject immediately with zero crypto work.

`strings.TrimPrefix(authHeader, "Bearer ")` strips the scheme prefix. The token string is everything after the space.

`jwt.ParseWithClaims` validates three things: signature (HMAC-SHA256 with `secret`), expiry (the `exp` claim in `RegisteredClaims`), and algorithm.

`jwt.WithValidMethods([]string{"HS256"})` is non-negotiable. Without it, an attacker can craft a JWT with `"alg":"none"` in the header:

```
Malicious header (base64-decoded): {"alg":"none","typ":"JWT"}
Malicious payload:                 {"iss":"attacker","exp":9999999999}
Signature:                         (empty string)
```

A library without algorithm validation will accept this token because `alg: none` means "no signature required." The HMAC verification is skipped. Any request with this token passes as authenticated.

With `jwt.WithValidMethods([]string{"HS256"})`, the library checks the `alg` header field before attempting verification. `"none"` is not in the list; the parse fails; the middleware returns 401.

The same attack works with algorithm substitution (sending an RS256 public key as if it were an HS256 secret), which `WithValidMethods` also blocks.

---

## 8. The Repository Pattern

`internal/server/storage/metrics.go` defines the interface and implementation:

```go
// MetricRepository defines storage operations for metrics.
type MetricRepository interface {
    Insert(ctx context.Context, batch model.MetricBatch) (int, error)
    Query(ctx context.Context, params QueryParams) ([]QueryPoint, error)
    Ping(ctx context.Context) error
}

type pgxMetricRepository struct {
    pool *pgxpool.Pool
}

// NewMetricRepository creates a MetricRepository backed by a pgxpool.
func NewMetricRepository(pool *pgxpool.Pool) MetricRepository {
    return &pgxMetricRepository{pool: pool}
}
```

**Why does the interface live in the storage package, next to its implementation?** Go interfaces are defined where they are *used*, not where they are *implemented*. The `api` package imports `storage` and calls `storage.MetricRepository`. That is where the contract is defined. This is the opposite of Java-style `IMetricRepository` in a separate `interfaces` package. The Go approach means the interface stays close to the consumers who depend on it.

**`pgxMetricRepository` is unexported.** Only `NewMetricRepository` is public. All callers receive a `MetricRepository` interface value. This means you can swap the implementation (to a SQLite backend for local dev, or to a test double) without changing any caller. The `fakeRepo` in `testhelpers_test.go` is exactly this: it implements `MetricRepository` with no DB at all.

**Batch insert with `pgx.Batch`.** The insert method queues all rows into a single batch and sends them in one round-trip:

```go
func (r *pgxMetricRepository) Insert(ctx context.Context, batch model.MetricBatch) (int, error) {
    b := &pgx.Batch{}
    const q = `INSERT INTO metrics (time, host, name, value, labels) VALUES ($1,$2,$3,$4,$5)`
    for _, m := range batch.Metrics {
        // ...
        b.Queue(q, m.Time, m.Host, m.Name, m.Value, labels)
    }
    br := r.pool.SendBatch(ctx, b)
    defer br.Close() // CRITICAL: must close to release pool connection
    for i := 0; i < b.Len(); i++ {
        if _, err := br.Exec(); err != nil {
            return i, fmt.Errorf("insert metric[%d]: %w", i, err)
        }
    }
    return b.Len(), nil
}
```

Without `pgx.Batch`, inserting 50 metrics means 50 separate round-trips to PostgreSQL: 50 × (network RTT + query parse + query execute + response). With `pgx.Batch`, it is 1 round-trip that carries all 50 queries.

**`defer br.Close()` is not optional.** `SendBatch` reserves a connection from the pool for the duration of the batch result set. If you return without calling `br.Close()`, that connection is never returned to the pool. Under load, all connections exhaust and every subsequent request hangs waiting for a connection. This is the single most common pgx bug. The `defer` on the line immediately after `SendBatch` ensures it always runs regardless of early returns.

**Why no ORM?** The `Query` method uses `time_bucket`, a TimescaleDB-specific aggregate function for time-series bucketing. No ORM knows about this function. Raw SQL gives you full access to the database's capabilities. The query also uses allowlist-interpolated identifiers for `bucket` and `aggFn` (see `validBuckets` and `validAggs` maps), which is safe because those values come exclusively from the maps, never from raw user input.

---

## 9. Dynamic Query Builder

When building queries with optional WHERE clauses, never build strings by concatenating user input. The pattern used in this codebase builds the clause structure in Go and passes values as bound parameters:

```go
var conds []string
var args []any
n := 0
add := func(cond string, val any) {
    n++
    conds = append(conds, fmt.Sprintf(cond, n))
    args = append(args, val)
}

if params.Host != "" {
    add("host = $%d", params.Host)
}
if params.Level != "" {
    add("level = $%d", params.Level)
}

whereClause := ""
if len(conds) > 0 {
    whereClause = "WHERE " + strings.Join(conds, " AND ")
}
q := fmt.Sprintf("SELECT * FROM logs %s ORDER BY time DESC", whereClause)
rows, err := pool.Query(ctx, q, args...)
```

**Why this is injection-safe:** The `$N` placeholder in `cond` is a positional parameter reference, not the value. The value goes into `args`. PostgreSQL receives the query structure and the values separately. An attacker who controls `params.Level` can inject anything into `args[n]`, but PostgreSQL treats it as a literal string value to compare against `level`, not as SQL syntax. The WHERE clause structure itself is built entirely from Go string literals (`"host = $%d"`, `"level = $%d"`), which are not user-controlled.

This approach scales to any number of optional filters. Each new filter adds one call to `add()`.

---

## 10. Testing Philosophy

MiniObserv uses three testing layers with a clear separation of concerns.

### Layer 1 — Pure unit tests (no I/O)

No network, no database, no real OS calls. These run in milliseconds and have zero setup.

**Example: `TestQueryParams_Validate` in `storage/metrics_test.go`**

```go
func TestQueryParams_Validate(t *testing.T) {
    base := storage.QueryParams{
        Host:   "host1",
        Name:   "cpu.usage_pct",
        From:   time.Now().Add(-time.Hour),
        To:     time.Now(),
        Bucket: "1m",
        Agg:    "avg",
    }
    t.Run("valid", func(t *testing.T) {
        if err := base.Validate(); err != nil {
            t.Fatalf("expected nil, got %v", err)
        }
    })
    t.Run("invalid bucket", func(t *testing.T) {
        p := base
        p.Bucket = "2m"
        if err := p.Validate(); err == nil {
            t.Fatal("expected error")
        }
    })
    // ...
}
```

This tests pure Go logic — the allowlist lookups, time comparison, range check. No database is involved.

### Layer 2 — httptest handler tests

These test the full handler behavior — parsing, validation, error responses, status codes — using `httptest.NewRecorder()`. No real HTTP connection, no real database.

The `fakeRepo` in `testhelpers_test.go` is a test double that satisfies `storage.MetricRepository`:

```go
type fakeRepo struct {
    pingErr   error
    insertN   int
    insertErr error
    queryPts  []storage.QueryPoint
    queryErr  error
}

func (f *fakeRepo) Insert(_ context.Context, _ model.MetricBatch) (int, error) {
    return f.insertN, f.insertErr
}
func (f *fakeRepo) Query(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
    return f.queryPts, f.queryErr
}
func (f *fakeRepo) Ping(_ context.Context) error { return f.pingErr }
```

A handler test:

```go
func TestHandleIngest_ValidBatch(t *testing.T) {
    repo := &fakeRepo{insertN: 3}
    handler := api.HandleIngest(repo)

    batch := makeBatch("web-01", 3)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics", encodeJSON(t, batch))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    handler.ServeHTTP(rr, req)

    if rr.Code != http.StatusAccepted {
        t.Fatalf("expected 202, got %d", rr.Code)
    }
    var resp map[string]int
    mustDecode(t, rr.Body, &resp)
    if resp["ingested"] != 3 {
        t.Fatalf("expected ingested=3, got %d", resp["ingested"])
    }
}
```

`httptest.NewRequest` builds an `*http.Request` without a real network connection. `httptest.NewRecorder` captures what the handler writes. The test reads the status code and response body directly.

### Layer 3 — Integration tests

These run against a real TimescaleDB instance and are guarded by a build tag:

```go
//go:build integration

package storage_test

import (
    "os"
    "testing"
)

func TestMetricRepository_Integration(t *testing.T) {
    dbURL := os.Getenv("TEST_DATABASE_URL")
    if dbURL == "" {
        t.Skip("TEST_DATABASE_URL not set — skipping integration test")
    }
    // real pgx, real SQL, real TimescaleDB
}
```

The `//go:build integration` tag at the top of the file means the file is excluded from the default build. Running `go test ./...` never touches this file. CI runs `go test -tags=integration ./...` with a real TimescaleDB container spun up for the job.

The double guard (`build tag` + `t.Skip` when env var is missing) means even if someone runs with `-tags=integration` locally without a database, the test gracefully skips instead of failing with a connection error.

---

## 11. Adding a New Endpoint (Step by Step)

Concrete example: add `GET /api/v1/metrics/hosts` that returns a list of all unique hosts in the metrics table.

### Step 1 — Add the method to the interface

In `internal/server/storage/metrics.go`, add `Hosts` to `MetricRepository`:

```go
type MetricRepository interface {
    Insert(ctx context.Context, batch model.MetricBatch) (int, error)
    Query(ctx context.Context, params QueryParams) ([]QueryPoint, error)
    Ping(ctx context.Context) error
    Hosts(ctx context.Context) ([]string, error)  // new
}
```

### Step 2 — Implement it on `pgxMetricRepository`

Still in `metrics.go`:

```go
func (r *pgxMetricRepository) Hosts(ctx context.Context) ([]string, error) {
    rows, err := r.pool.Query(ctx, `SELECT DISTINCT host FROM metrics ORDER BY host`)
    if err != nil {
        return nil, fmt.Errorf("query hosts: %w", err)
    }
    defer rows.Close()

    var hosts []string
    for rows.Next() {
        var h string
        if err := rows.Scan(&h); err != nil {
            return nil, fmt.Errorf("scan host: %w", err)
        }
        hosts = append(hosts, h)
    }
    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("iterate hosts: %w", err)
    }
    return hosts, nil
}
```

### Step 3 — Create the handler

In `internal/server/api/metrics.go`:

```go
// HandleHosts returns a handler for GET /api/v1/metrics/hosts.
func HandleHosts(repo storage.MetricRepository) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        hosts, err := repo.Hosts(r.Context())
        if err != nil {
            writeError(w, http.StatusInternalServerError, "query error")
            return
        }
        if hosts == nil {
            hosts = []string{} // return [] not null
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]any{"hosts": hosts})
    }
}
```

### Step 4 — Register the route

In `internal/server/api/router.go`, inside the JWT group:

```go
r.Group(func(r chi.Router) {
    r.Use(JWTMiddleware(jwtSecret))
    r.Post("/api/v1/metrics", HandleIngest(repo))
    r.Get("/api/v1/metrics/query", HandleQuery(repo))
    r.Get("/api/v1/metrics/hosts", HandleHosts(repo))  // new
})
```

### Step 5 — Add the stub to `fakeRepo`

In `internal/server/api/testhelpers_test.go`:

```go
type fakeRepo struct {
    pingErr   error
    insertN   int
    insertErr error
    queryPts  []storage.QueryPoint
    queryErr  error
    hosts     []string   // new
    hostsErr  error      // new
}

func (f *fakeRepo) Hosts(_ context.Context) ([]string, error) {
    return f.hosts, f.hostsErr
}
```

### Step 6 — Write the handler test

In `internal/server/api/metrics_test.go`:

```go
func TestHandleHosts_ReturnsList(t *testing.T) {
    repo := &fakeRepo{hosts: []string{"web-01", "web-02"}}
    handler := api.HandleHosts(repo)

    req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/hosts", nil)
    rr := httptest.NewRecorder()
    handler.ServeHTTP(rr, req)

    if rr.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rr.Code)
    }
    var resp map[string][]string
    mustDecode(t, rr.Body, &resp)
    if len(resp["hosts"]) != 2 {
        t.Fatalf("expected 2 hosts, got %d", len(resp["hosts"]))
    }
}

func TestHandleHosts_EmptyList(t *testing.T) {
    repo := &fakeRepo{hosts: nil}
    handler := api.HandleHosts(repo)

    req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/hosts", nil)
    rr := httptest.NewRecorder()
    handler.ServeHTTP(rr, req)

    if rr.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rr.Code)
    }
    var resp map[string][]string
    mustDecode(t, rr.Body, &resp)
    // Must be [] not null
    if resp["hosts"] == nil {
        t.Fatal("expected non-nil empty array, got null")
    }
}
```

The full path: interface → implementation → handler → router → test double → test. No step is optional; skipping the interface change breaks the `fakeRepo` compilation; skipping the router registration means the handler never receives requests.

---

## 12. Configuration Pattern

`internal/config/agent.go` loads all configuration from environment variables:

```go
func LoadAgent() (AgentConfig, error) {
    rawURL := os.Getenv("SERVER_URL")
    if rawURL == "" {
        return AgentConfig{}, fmt.Errorf("SERVER_URL is required but not set")
    }
    // ...

    interval := 10 * time.Second
    if raw := os.Getenv("COLLECT_INTERVAL"); raw != "" {
        if d, parseErr := time.ParseDuration(raw); parseErr == nil && d >= time.Second && d <= 300*time.Second {
            interval = d
        }
        // out-of-range or parse error: silently fall back to default
    }
    // ...
}
```

**Every setting from env, never flags or config files.** This is the [12-factor app](https://12factor.net/config) principle. The binary's behavior is fully determined by the environment it runs in. No config file to find, no flag parsing to debug, no difference between local dev and production except the env vars set.

**Required vars fail-fast.** `SERVER_URL` (agent) and `DATABASE_URL` + `AGENT_TOKEN` (server) are required. If they are missing, `LoadAgent` or `LoadServerConfig` returns an error. `cmd/agent/main.go` treats this as fatal:

```go
cfg, err := config.LoadAgent()
if err != nil {
    slog.Error("invalid configuration", "err", err)
    os.Exit(1)
}
```

This happens at process start, before any goroutines launch. The error message tells the operator exactly which variable is missing. No panic, no nil-pointer dereference later, no subtle wrong-behavior — immediate exit with a clear message.

**Optional vars fall back to safe defaults silently.** `COLLECT_INTERVAL`, `LOG_LEVEL`, `LISTEN_ADDR` — if unset or invalid, the code uses a hardcoded default without logging a warning. The rationale: these have sensible defaults. Logging a warning for every unset optional var would create noise that buries real errors.

**Duration bounds matter.** The `parseDuration` helper in `config/server.go`:

```go
func parseDuration(raw string, def, min, max time.Duration) time.Duration {
    if raw == "" {
        return def
    }
    d, err := time.ParseDuration(raw)
    if err != nil || d < min || d > max {
        return def
    }
    return d
}
```

`COLLECT_INTERVAL` is bounded to `[1s, 300s]`. Without the lower bound, `COLLECT_INTERVAL=0` would create a zero-duration `time.NewTicker`, which panics in Go's stdlib (`panic: non-positive interval for NewTicker`). Without the upper bound, `COLLECT_INTERVAL=24h` would mean the server receives no data for 24 hours and appears unresponsive. Both are misconfiguration, not intent — bounds catch them and fall back to the safe default.

The same `parseDuration` is used for `REQUEST_TIMEOUT` and `SHUTDOWN_TIMEOUT` on the server side, each with appropriate bounds for their use case.
