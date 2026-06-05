# Architecture Decision Records

This file documents the key technical decisions made during development of MiniObserv. Each record explains the context that forced the decision, what was decided, and the resulting trade-offs.

---

## ADR-1: HTTP/JSON over gRPC for metric transport

**Status**: Accepted
**Date**: 2026-06-05

### Context

Observability agents typically use one of two transport models: gRPC (protobuf, streaming, multiplexed) or HTTP/JSON (text, request/response). Both can handle the expected workload of tens of agents each pushing a batch every 10 seconds.

### Decision

Use HTTP/JSON with batched POST requests (`POST /api/v1/metrics`). Agents accumulate metrics for one collection interval and send them as a single JSON body.

### Consequences

- **No protobuf schema**: adding or renaming a field requires no `.proto` compilation step; the Go struct is the schema.
- **Easier debugging**: `curl` and any HTTP client can inspect, replay, and test the ingest endpoint without generated stubs.
- **Slightly higher payload size** than protobuf for the same data, but at 10-second intervals and small batches the difference is negligible. The bottleneck is database write throughput, not transport bandwidth.
- **No server-push or streaming**: agents always initiate; the server cannot request data on demand. Acceptable for a push-based model.
- gRPC becomes relevant only if MiniObserv needs bidirectional streaming, multiplexed agent control, or strict schema enforcement across teams.

---

## ADR-2: Flat monorepo with a single go.mod

**Status**: Accepted
**Date**: 2026-06-05

### Context

The project contains two binaries (agent, server) and shared code (`internal/model`, `internal/config`). Go workspaces (`go work`) allow splitting into multiple modules within a single repository.

### Decision

Use a single `go.mod` at the repository root. Both binaries and all internal packages share one module (`github.com/kamerrezz/theminidog`).

### Consequences

- **One `go.sum`**: all dependency versions are resolved once; no cross-module version skew.
- **Simple tooling**: `go build ./...`, `go test ./...`, and `go vet ./...` cover the entire project without workspace flags.
- **Shared `internal/`**: Go's `internal` visibility rule applies at the module boundary; no extra configuration needed to share code.
- If the agent and server were ever to be deployed from separate repositories or by separate teams with different release cadences, splitting into modules (or a workspace) would become necessary.

---

## ADR-3: Narrow metric model — single table, 9 canonical names

**Status**: Accepted
**Date**: 2026-06-05

### Context

Time-series data for infrastructure metrics is commonly stored either as wide tables (one column per metric type) or as narrow/event tables (one row per measurement with a name column). The schema must support both write-heavy ingestion and time-range aggregation queries.

### Decision

Use a single narrow table:

```sql
CREATE TABLE metrics (
    time   TIMESTAMPTZ      NOT NULL,
    host   TEXT             NOT NULL,
    name   TEXT             NOT NULL,
    value  DOUBLE PRECISION NOT NULL,
    labels JSONB
);
```

Restrict `name` to exactly 9 allowlisted values enforced at validation time in `internal/model` and `internal/server/storage`.

### Consequences

- **Adding a new metric type** requires only: (1) add the name to the allowlist in both packages, (2) implement a collector. No `ALTER TABLE` needed.
- **TimescaleDB hypertable** works naturally on the narrow model: time-bucketed aggregations use `time_bucket(interval, time)` regardless of metric type.
- **JSONB labels** store per-measurement context (`{"core": "0"}`, `{"mount": "/data"}`) without sparse nullable columns polluting the schema.
- **Value is always a single `DOUBLE PRECISION`**: metrics that are naturally multi-dimensional (e.g., CPU usage per core) are stored as multiple rows with different label values. This is idiomatic for time-series storage.
- A query for a specific metric + host is efficient: the composite index `(host, name, time DESC)` covers the most common access pattern.

---

## ADR-4: pgx/v5 with no ORM

**Status**: Accepted
**Date**: 2026-06-05

### Context

Go ORM libraries (GORM, sqlc, ent) can reduce boilerplate for CRUD operations on relational data. However, MiniObserv's storage layer has two unusual requirements: bulk inserts via `pgx.Batch` and TimescaleDB-specific functions (`time_bucket`, `create_hypertable`).

### Decision

Use `pgx/v5` directly with raw SQL. No ORM, no query builder.

### Consequences

- **Full `pgx.Batch` access**: bulk-inserting a batch of metrics in a single round-trip is natural with `pgx.Batch`; ORMs either don't expose this or generate per-row round-trips.
- **`time_bucket()` in queries**: TimescaleDB's aggregation functions are first-class SQL. An ORM would require escape hatches (raw SQL blocks) for every query that uses them, negating the abstraction benefit.
- **More verbose insert code**: the `Insert` method in `storage/metrics.go` manually queues each metric into the batch. This is acceptable given the simple schema.
- Migration: `golang-migrate` manages schema changes; no ORM migration engine is needed.

---

## ADR-5: pgx.Batch for bulk inserts

**Status**: Accepted
**Date**: 2026-06-05

### Context

Each collection tick produces a batch of metrics (typically 10–50 rows). Naively, inserting each row in a separate `INSERT` statement means N round-trips per tick. At 10-second intervals with multiple agents, this multiplies DB load unnecessarily.

### Decision

Use `pgx.Batch` to pipeline all `INSERT` statements in a single round-trip:

```go
b := &pgx.Batch{}
for _, m := range batch.Metrics {
    b.Queue(insertSQL, m.Time, m.Host, m.Name, m.Value, labels)
}
br := r.pool.SendBatch(ctx, b)
defer br.Close() // CRITICAL: must close to release the pool connection
for i := 0; i < b.Len(); i++ {
    if _, err := br.Exec(); err != nil { ... }
}
```

### Consequences

- **One network round-trip per batch** regardless of batch size (up to `maxBatchSize = 1000`).
- **`defer br.Close()` is mandatory**: if the `BatchResults` is not closed, the connection borrowed from `pgxpool` is never returned. This causes the pool to exhaust over time.
- **Partial failure handling**: if one `Exec()` call fails, the loop returns immediately with the failing index. Rows queued after the failure point are not inserted; rows before it are committed (PostgreSQL auto-commit per statement unless in an explicit transaction). This is acceptable for metric ingestion where partial loss is preferable to blocking.
- If transactional all-or-nothing semantics are ever required, wrap the batch in an explicit `BEGIN`/`COMMIT`.

---

## ADR-6: time_bucket allowlist interpolation (Option B) for query parameters

**Status**: Accepted
**Date**: 2026-06-05

### Context

The query handler accepts `bucket` (e.g., `"5m"`) and `agg` (e.g., `"avg"`) parameters that must appear in the SQL sent to TimescaleDB. The natural pgx approach would use a prepared statement parameter (`$1::interval`) for the bucket value. However, pgx caches prepared statement plans by query text; a parameterised `$1::interval` causes plan cache collisions between requests with different interval types.

### Decision

Use allowlist-based string interpolation (Option B). Both `bucket` and `agg` values are validated against explicit maps before interpolation:

```go
var validBuckets = map[string]string{
    "1m": "1 minute", "5m": "5 minutes", "15m": "15 minutes",
    "1h": "1 hour",   "1d": "1 day",
}
var validAggs = map[string]string{
    "avg": "avg", "max": "max", "min": "min",
}

q := fmt.Sprintf(`
    SELECT time_bucket('%s', time) AS bucket, %s(value) AS value
    FROM metrics WHERE host=$1 AND name=$2 AND time>=$3 AND time<=$4
    GROUP BY bucket ORDER BY bucket DESC`,
    validBuckets[params.Bucket], validAggs[params.Agg],
)
```

The user-supplied strings never reach `fmt.Sprintf`; only the pre-validated SQL-safe literals do.

### Consequences

- **SQL injection is impossible**: the interpolated values come exclusively from the allowlist maps, never from raw user input.
- **No prepared-statement plan cache conflict**: each `(bucket, agg)` combination produces a distinct query string. pgx caches plans per query text, so these are separate cache entries.
- **Dynamic query strings are harder to test statically**: adding a new valid bucket requires updating the map; a typo in the map value would reach the DB. The allowlist is small and manually reviewed.
- Option A (parameterized `$1::interval`) was rejected because it causes pgx plan-cache issues. Option C (disabling prepared statements entirely) was rejected as it loses all caching benefits.

---

## ADR-7: golang-migrate with the pgx5:// DSN scheme

**Status**: Accepted
**Date**: 2026-06-05

### Context

`golang-migrate` supports multiple database drivers. When using PostgreSQL with pgx/v5, two driver packages exist: `database/pgx` (pgx v4) and `database/pgx/v5` (pgx v5). The driver registration determines which DSN scheme is recognized.

### Decision

Import `_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"` and rewrite the DSN scheme from `postgres://` to `pgx5://` before passing it to `migrate.New`:

```go
migrateURL := strings.Replace(cfg.DatabaseURL, "postgres://", "pgx5://", 1)
migrateURL = strings.Replace(migrateURL, "postgresql://", "pgx5://", 1)
m, err := migrate.New(fmt.Sprintf("file://%s", cfg.MigrationsPath), migrateURL)
```

`pgxpool.New` still receives the original `postgres://` DSN (separate code path).

### Consequences

- Using `postgres://` with the pgx/v5 driver registration causes a "no registered driver" panic at startup — the most common misconfiguration in this setup.
- The DSN rewrite is done once at startup; the original `DATABASE_URL` value is used everywhere else (pgxpool, logging).
- The pgx/v5 migrate driver and the pgxpool use separate connections; there is no conflict.

---

## ADR-8: TimescaleDB extension before create_hypertable in migrations

**Status**: Accepted
**Date**: 2026-06-05

### Context

`create_hypertable` is a function provided by the TimescaleDB extension. If the extension is not loaded when the migration runs, the call fails with "function create_hypertable does not exist".

### Decision

The first (and currently only) migration file ensures the extension is created before the hypertable:

```sql
CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

CREATE TABLE IF NOT EXISTS metrics ( ... );

SELECT create_hypertable('metrics', 'time',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);
```

`CASCADE` is required because TimescaleDB installs internal functions and types that depend on each other.

### Consequences

- The migration is idempotent: `IF NOT EXISTS` guards on both `CREATE EXTENSION` and `create_hypertable` mean re-running the migration (e.g., after a failed run) is safe.
- The database user running migrations must have `SUPERUSER` or `CREATE EXTENSION` privilege. In managed PostgreSQL environments (e.g., AWS RDS), verify that TimescaleDB is enabled as an available extension before deploying.
- If TimescaleDB is not installed on the PostgreSQL instance, the migration fails immediately with a clear error.

---

## ADR-9: JWT HS256 shared secret (AGENT_TOKEN)

**Status**: Accepted
**Date**: 2026-06-05

### Context

The ingest and query endpoints must be protected from unauthorized access. Options considered:

1. **API key** (static secret in header) — simple, but no expiry, harder to rotate
2. **JWT HS256 shared secret** — signed tokens with expiry, both sides use the same secret
3. **JWT RS256 asymmetric** — agent has private key, server has public key; more complex key management

### Decision

Use JWT HS256 with a single shared secret (`AGENT_TOKEN`). The agent mints a 24-hour token on startup; the server validates it with `jwt.WithValidMethods([]string{"HS256"})`.

### Consequences

- **Simplicity**: one secret to configure, same value for both agent and server.
- **Token expiry**: tokens expire after 24 hours. If an agent runs for more than 24 hours without restart, its token expires and requests are rejected with 401. The fix is to restart the agent (which re-mints a fresh token). Long-running token refresh is a known gap.
- **Shared secret risk**: if `AGENT_TOKEN` is leaked, any party can mint valid tokens. Treat it as a high-value secret.
- **`WithValidMethods` is non-negotiable**: without it, an attacker can forge tokens using `alg: none` (no signature) or a weak algorithm. The middleware explicitly rejects any token whose algorithm is not `"HS256"`.
- RS256 is a natural next step if the agent and server need to be operated by different parties with key isolation.

---

## ADR-10: WithToken() functional option on HTTPSender

**Status**: Accepted
**Date**: 2026-06-05

### Context

`NewHTTPSender` needs to optionally carry a Bearer token. Adding `token string` as a positional parameter to the constructor would break all call sites if the signature is extended later. An options struct is an alternative but adds boilerplate for a single optional field.

### Decision

Use a functional option method that returns `*HTTPSender` for chaining:

```go
func (s *HTTPSender) WithToken(token string) *HTTPSender {
    s.token = token
    return s
}

// Usage:
snd := sender.NewHTTPSender(url, backoffCfg, log).WithToken(agentToken)
```

### Consequences

- **Stable constructor signature**: `NewHTTPSender(url, backoff, log)` never changes regardless of how many optional parameters are added.
- **Chaining is explicit**: callers read `NewHTTPSender(...).WithToken(tok)` and immediately understand that the token is optional.
- **Not zero-value safe** if misused: calling `WithToken("")` intentionally suppresses the header (empty token = no `Authorization` header sent). This is the desired behavior for unauthenticated deployments.
- The pattern can be extended: `WithTimeout`, `WithRetries`, etc. follow the same convention without changing `NewHTTPSender`.

---

## ADR-11: statFn injection on all four collectors

**Status**: Accepted
**Date**: 2026-06-05

### Context

All four collectors (CPU, memory, disk, network) use gopsutil to read OS metrics. gopsutil makes real syscalls. Unit tests that call the real gopsutil functions are slow, flaky on CI (metrics vary), and require OS-specific behavior.

### Decision

Each collector stores its OS interaction function as a struct field (`statFn`, `partFn`, `usageFn`, `ioFn`). Production constructors (`NewCPUCollector`, etc.) wire the real gopsutil function. Tests replace the field with a deterministic stub:

```go
// Production
c := &CPUCollector{
    host:   host,
    statFn: gopsutilcpu.PercentWithContext, // real gopsutil
}

// Test
c := &CPUCollector{
    host: "test-host",
    statFn: func(ctx context.Context, interval time.Duration, percpu bool) ([]float64, error) {
        return []float64{42.0}, nil // controlled, deterministic
    },
}
```

### Consequences

- **OS-free unit tests**: tests run in milliseconds, produce consistent results, and work identically on Linux, macOS, and Windows.
- **Full coverage of business logic**: error paths, empty results, and edge cases (negative deltas for network, zero total for disk) can all be exercised without relying on OS state.
- **Slight coupling to gopsutil types**: `NetworkCollector.ioFn` uses `gopsutilnet.IOCountersStat` in its signature. Abstracting this further would require an additional interface layer with no practical benefit at this scale.
- This is dependency injection at the function level. The same goal could be achieved with an interface, but function fields are simpler when there is exactly one operation to inject.

---

## ADR-12: Delta semantics for network metrics

**Status**: Accepted
**Date**: 2026-06-05

### Context

The OS exposes network I/O as monotonically increasing counters (total bytes received/sent since boot). Storing raw counter values is not useful for dashboards that show traffic rate over time. Two options exist: (1) compute deltas in the agent before storing, (2) store raw counters and compute deltas at query time.

### Decision

Compute deltas in the agent. `NetworkCollector` keeps a `prev` snapshot. On the first call it seeds `prev` and returns nil. On subsequent calls it returns `bytes_current - bytes_prev` per interface.

Negative deltas (counter wrap or agent restart) are clamped to zero.

### Consequences

- **Stored values are immediately meaningful**: a `net.bytes_in` row with value `4096` means 4096 bytes were received in the last collection interval, regardless of when the agent started.
- **First tick is always empty**: there is no previous snapshot on startup, so the first batch from any agent contains no network metrics. This is documented behavior (see architecture.md §9).
- **Counter reset appears as a zero**: if the OS counter resets (reboot, overflow), the delta is negative and clamped to zero. The series will show a momentary dip to zero. This is less confusing than a sudden spike to a large positive value.
- If raw cumulative counters are ever needed (e.g., for billing or total transfer accounting), they would require a separate metric name or a different storage model. The current design does not preserve them.

---

## ADR-13: Channel drop-newest policy for the batch buffer

**Status**: Accepted
**Date**: 2026-06-05

### Context

The `Agent` uses a buffered channel (`make(chan model.MetricBatch, 10)`) to decouple the collection goroutine from the sender goroutine. If the sender is slow (e.g., server is down, backoff in progress) and the collection loop keeps producing batches, the channel will fill up.

Two options for handling a full channel:
- **Drop oldest**: overwrite the oldest entry in the ring buffer (requires a more complex data structure)
- **Drop newest**: discard the incoming batch (non-blocking send on a full channel)

### Decision

Use a non-blocking channel send that discards the incoming batch when the channel is full:

```go
select {
case a.batches <- batch:
default:
    a.log.WarnContext(ctx, "batch channel full, dropping")
}
```

### Consequences

- **Oldest data is preserved**: if the server is temporarily unreachable and the sender is retrying with backoff, the oldest queued batches will be delivered first when the connection recovers. Recent data is lost, but the historical baseline is maintained.
- **No blocking**: the collection loop never stalls waiting for the sender. If metrics cannot be delivered, collection continues normally.
- **Buffer size 10** provides approximately 100 seconds of buffering at the default 10-second interval. This is enough to survive short server restarts.
- If data loss is unacceptable, the correct solution is a persistent write-ahead buffer (disk-backed queue), not a larger in-memory channel.

---

## ADR-14: Migrations path in Docker via COPY, not go:embed

**Status**: Accepted
**Date**: 2026-06-05

### Context

`golang-migrate` supports two migration source drivers: `file://` (reads from the filesystem) and `iofs://` (reads from an `io.FS`, enabling `//go:embed`). Embedding migrations in the binary is convenient for single-binary deployment. However, it requires a `tools.go` or similar workaround to ensure the `embed` directive includes the migration files in the binary.

### Decision

Use the `file://` source driver. In Docker, copy migration files explicitly:

```dockerfile
WORKDIR /app
COPY migrations/ /app/migrations
```

Set `MIGRATIONS_PATH=/app/migrations` (or use the default `./migrations` with the working directory set correctly).

### Consequences

- **Simpler Dockerfile**: no `//go:embed` directives, no `tools.go` dependencies.
- **Migrations are visible on disk** inside the container, which simplifies debugging (exec into the container and inspect `.sql` files directly).
- **Deployment must always include the migrations directory**: a binary-only deployment without the migrations folder will fail at startup. `//go:embed` would have baked them in.
- The environment variable `MIGRATIONS_PATH` allows overriding the path without rebuilding the image — useful for local development where the working directory may differ from the container layout.
- If single-binary deployment with embedded migrations is ever required, switching to `iofs://` with `//go:embed` is a self-contained change to `cmd/server/main.go`.

---

## ADR-15: WithNotifiers functional option on Evaluator

**Status**: Accepted
**Date**: 2026-06-05

### Context

Week 5 added a webhook notification system. Notifiers needed to be injected into the server without breaking the 213 existing tests that instantiate the server directly. Adding a positional parameter to the constructor would have required updating every test call site.

### Decision

Add a functional option `WithNotifiers(notifiers []Notifier) ServerOption` that is passed to the server constructor only when notifiers are configured. Existing tests require no changes.

### Consequences

- **Stable constructor signature**: `NewServer(...)` never changes regardless of how many optional parameters are added in future.
- **Backward compatibility**: all 213 existing tests continue to compile and pass without modification.
- **Extensible**: adding new notifier types (email, PagerDuty) only requires implementing the `Notifier` interface.
- **Silent default**: if `WithNotifiers` is not called, notification behavior is a no-op. This is correct for unit tests but would be a misconfiguration in production if notifiers were expected.
- The functional options pattern is the idiomatic Go approach for extending constructors with optional parameters without breaking existing callers.

---

## ADR-16: In-memory HostTracker, no DB persistence

**Status**: Accepted
**Date**: 2026-06-05

### Context

The system needs to know whether a host is alive, stale, or down. Two approaches were considered: persisting liveness state to the database, or maintaining it purely in memory.

### Decision

`HostTracker` is an in-memory struct with a `map[string]time.Time` protected by a mutex. The host heartbeat is updated on every metrics ingest. Nothing is written to the database.

### Consequences

- **Zero-latency updates**: no database round-trip on every ingest tick.
- **No additional DB load**: TimescaleDB is not involved in liveness tracking.
- **State is lost on restart**: after a server restart all hosts appear as `unknown` until they send their first batch. In production this can generate false `host.down` alerts at startup.
- **Liveness is ephemeral by design**: a server restart repopulates state within the stale window (typically seconds to minutes). The operational cost of persisting this state does not justify the benefit, especially in v1 with a single server.

---

## ADR-17: Heartbeat at the ingest handler, not the storage layer

**Status**: Accepted
**Date**: 2026-06-05

### Context

A decision was needed about where in the ingest flow to update the host's last-seen timestamp: in the HTTP handler or in the storage repository.

### Decision

`HandleIngest` calls `hostTracker.Heartbeat(host)` immediately after validating the batch and before calling `storage.Insert`. The storage repository has no knowledge of `HostTracker`.

### Consequences

- **Clear separation of concerns**: `storage` remains focused on persistence; liveness tracking belongs to the API layer.
- **Heartbeat recorded even on storage failure**: if the database insert fails after the heartbeat, the host is still considered alive — which is correct, as it did send a valid batch.
- **Invalid batches do not update liveness**: validation happens before the heartbeat call, so a malformed batch does not reset the host's timer.
- Placing the heartbeat in the storage layer would couple two unrelated concerns and make the storage package harder to test in isolation.

---

## ADR-18: Hardcoded retention intervals v1

**Status**: Accepted
**Date**: 2026-06-05

### Context

TimescaleDB supports automatic data retention policies (`add_retention_policy`). The question was whether to expose retention intervals as environment variables or hardcode them for v1.

### Decision

Retention intervals are defined as constants in code for v1 (metrics: 30 days, logs: 14 days). They are not exposed as environment variables or configurable parameters.

### Consequences

- **Simple deployment**: no additional configuration required; behavior is predictable.
- **Straightforward migrations**: the retention policy SQL is a simple constant; no env-var interpolation or runtime configuration needed.
- **No operator flexibility**: changing intervals requires recompiling. Operators cannot tune retention without modifying source code.
- Exposing retention as configuration adds API surface and validation complexity. For v1, the defaults are sufficient; configurability can be added in v2 once real-world requirements are known.

---

## ADR-19: Logs hypertable conversion irreversible in down migration

**Status**: Accepted
**Date**: 2026-06-05

### Context

Migration 003 converts the `logs` table into a TimescaleDB hypertable via `create_hypertable`. TimescaleDB provides no mechanism to undo this conversion — there is no `drop_hypertable` that restores the original regular table.

### Decision

The `003_logs.down.sql` file contains only a comment explaining that the operation is irreversible. It does not attempt to recreate the table as a regular table or silently succeed while leaving the schema in an inconsistent state.

### Consequences

- **Operational honesty**: operators know before applying the migration that rollback requires a restore from backup or manual table recreation.
- **No silent failures**: a down migration that quietly does nothing is worse than one that documents its limitation explicitly.
- **Rollback requires a backup**: rolling back migration 003 means restoring from a pre-migration snapshot or recreating the `logs` table manually and reloading data.
- The down migration removes retention policies only (which can be re-added), and then stops. It does not touch the hypertable itself.

---

## ADR-20: Synthetic host.down via notifiers, not via ActiveAlerts()

**Status**: Accepted
**Date**: 2026-06-05

### Context

`host.down` events need to fire webhook notifications. One option was to model host-down as a special alert rule that flows through the existing `ActiveAlerts()` threshold evaluation system. The other was to have `HostTracker` fire notifiers directly.

### Decision

`HostTracker` fires notifiers directly when it detects that a host has exceeded `HOST_DOWN_AFTER`. The `host.down` event does not pass through the threshold-based alert system.

### Consequences

- **Decoupled systems**: threshold alerts (metric value crosses a boundary) and liveness alerts (host goes silent) can evolve independently.
- **host.down not in ActiveAlerts()**: if the alerts endpoint only exposes threshold alerts, `host.down` events will not appear there. This is intentional — they are a different class of event.
- **Simpler model**: a down host has no numeric value, does not "resolve" automatically when the metric returns, and is triggered by the absence of data — not its presence. Forcing it into the threshold model would require awkward special-casing.
- If a unified alert feed is ever needed, a shared event bus or a dedicated liveness-alert store would be the right approach, not repurposing `ActiveAlerts()`.

---

## ADR-21: Dashboard sidebar iterates HostStatuses with fallback to Hosts

**Status**: Accepted
**Date**: 2026-06-05

### Context

The dashboard sidebar needs to display the list of hosts with their current status. Data can come from two sources: `HostTracker.HostStatuses()` (real-time in-memory state) and the list of distinct hosts known in the database.

### Decision

The sidebar first iterates `HostTracker.HostStatuses()`. If the tracker has no entries — for example, after a server restart before any agent has reported — it falls back to the list of distinct hosts from the database.

### Consequences

- **UX continuity**: the sidebar shows a meaningful host list even immediately after a server restart, before the first heartbeat arrives.
- **Real-time state when available**: once agents start reporting, the sidebar reflects live liveness status.
- **Fallback hosts show no status**: during the fallback period all hosts appear as `unknown`. This is honest — the server genuinely does not know their current state.
- The alternative (showing an empty sidebar until the first heartbeat) would be confusing to users who expect to see their infrastructure immediately after a restart.

---

## ADR-22: Composite PK (id, time) required before create_hypertable on logs

**Status**: Accepted
**Date**: 2026-06-05

### Context

TimescaleDB requires that the partitioning column (`time`) be part of every UNIQUE constraint and the PRIMARY KEY before a table can be converted to a hypertable. The `logs` table originally had only `id` as its primary key.

### Decision

Migration 003 alters the `logs` table to use a composite primary key `(id, time)` before calling `create_hypertable('logs', 'time')`.

### Consequences

- **Correct hypertable conversion**: `create_hypertable` succeeds without constraint errors.
- **Future foreign-key references** to `logs.id` would also need to include `time` if added later.
- **This is a technical requirement, not a design choice**: omitting this step causes a migration-time error. The composite PK is the minimum change needed to satisfy TimescaleDB's constraint.
- Any UNIQUE index on `logs` must also include `time`. This is a permanent consequence of the hypertable conversion and must be kept in mind when adding future indexes.

---

## ADR-23: context.WithoutCancel for notification goroutines

**Status**: Accepted
**Date**: 2026-06-05

### Context

Webhook notifications are sent in independent goroutines. If the goroutines inherit the ingest handler's request context directly, they are cancelled as soon as the handler returns a response to the client — before the outbound HTTP request to the webhook can complete.

### Decision

Each notification goroutine receives `context.WithoutCancel(ctx)` instead of the original context. This decouples the goroutine's lifecycle from the HTTP request that triggered it. The goroutine retains its own 5-second deadline via `context.WithTimeout`.

### Consequences

- **Notifications complete independently**: even after the ingest handler has responded to the agent, the notification goroutine continues until it succeeds or times out.
- **5-second deadline is the only bound**: if the webhook endpoint is slow or unreachable, the goroutine waits up to 5 seconds before giving up.
- **SIGTERM edge case**: if the server receives SIGTERM while a notification is in flight, the goroutine will not be cancelled by the shutdown context — it will only stop when its own 5-second timeout expires. This is acceptable for a fire-and-forget notification system.
- `context.WithoutCancel` is the correct Go primitive for fire-and-forget operations that must outlive their caller's context.
