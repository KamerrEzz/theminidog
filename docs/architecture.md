# MiniObserv Architecture

## 1. System Overview

MiniObserv is a self-hosted, lightweight infrastructure observability platform. It collects host-level metrics (CPU, memory, disk, network) and stores them in a time-series database for later querying and visualization.

**What it is:**
- A pull-free metrics pipeline: agents push batches to the server on a fixed interval
- A storage backend designed for high-write, time-range-query workloads (TimescaleDB)
- A minimal API surface: ingest + time-bucketed query, nothing more

**What it is not:**
- A logs or traces collector (metrics only)
- A full observability platform with dashboards (no built-in UI)
- A distributed system: one server, N agents — no clustering or sharding

Module path: `github.com/kamerrezz/theminidog`
Go version: 1.23+

---

## 2. Component Diagram

```
┌──────────────────────────────────────────────────────────────┐
│  Host A                                                      │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  Agent (cmd/agent)                                     │  │
│  │                                                        │  │
│  │  CPUCollector ──┐                                      │  │
│  │  MemCollector ──┼──► collector.Registry ──► Agent ─── │──┼──► HTTP POST /api/v1/metrics
│  │  DiskCollector ─┤     (CollectAll)         (batches   │  │    Bearer JWT (HS256)
│  │  NetCollector ──┘                           channel)  │  │
│  └────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘

         (same pattern for Host B, Host C, ...)

┌──────────────────────────────────────────────────────────────┐
│  Server (cmd/server)                                         │
│                                                              │
│  chi Router                                                  │
│    GET  /healthz          (no auth)                          │
│    GET  /readyz           (no auth)                          │
│    POST /api/v1/metrics   (JWT required) ──► HandleIngest    │
│    GET  /api/v1/metrics/query (JWT required) ──► HandleQuery │
│                                                              │
│  JWTMiddleware ──► storage.MetricRepository                  │
│                         │                                    │
└─────────────────────────┼────────────────────────────────────┘
                          │
                          ▼
┌──────────────────────────────────────────────────────────────┐
│  TimescaleDB (PostgreSQL + timescaledb extension)            │
│                                                              │
│  Table: metrics (hypertable, partitioned by time, 1d chunks) │
│  Columns: time TIMESTAMPTZ, host TEXT, name TEXT,            │
│           value DOUBLE PRECISION, labels JSONB               │
│  Index: (host, name, time DESC)                              │
└──────────────────────────────────────────────────────────────┘
```

---

## 3. Data Flow

### Collection tick (every 10 s by default)

```
ticker fires
    │
    ▼
collector.Registry.CollectAll(ctx)
    │
    ├── CPUCollector.Collect()   → cpu.usage_pct  (core=total, core=0, core=1, ...)
    ├── MemoryCollector.Collect() → mem.used_pct, mem.used_bytes, mem.total_bytes
    ├── DiskCollector.Collect()  → disk.used_pct, disk.used_bytes, disk.total_bytes (per mount)
    └── NetworkCollector.Collect() → net.bytes_in, net.bytes_out (per iface, delta)
            │
            │  first call: seeds prev snapshot, returns nil
            │  subsequent calls: computes byte deltas
            ▼
[]model.Metric assembled into model.MetricBatch{Host, Metrics}
    │
    ▼  non-blocking channel send (drop-newest if channel full, buffer=10)
batches chan model.MetricBatch
    │
    ▼
HTTPSender.Send(ctx, batch)
    │
    ├── json.Marshal(batch)
    ├── http.NewRequestWithContext — POST <SERVER_URL>/api/v1/metrics
    ├── Authorization: Bearer <JWT>
    └── exponential backoff on 5xx / network errors
            │
            ▼ (HTTP 202 Accepted)
Server: JWTMiddleware validates HS256 token
    │
    ▼
HandleIngest:
    decode JSON → model.MetricBatch
    batch.Validate() — checks each Metric (host, name, time, finite value)
    maxBatchSize check (1000 metrics)
    │
    ▼
storage.pgxMetricRepository.Insert(ctx, batch)
    │
    ├── pgx.Batch — queues one INSERT per metric
    ├── pool.SendBatch(ctx, b) → single round-trip to DB
    ├── br.Exec() × len(batch)
    └── defer br.Close()  ← mandatory, releases pool connection
            │
            ▼
TimescaleDB hypertable: metrics
    time partitioned into 1-day chunks
    indexed on (host, name, time DESC)
```

### Query flow

```
GET /api/v1/metrics/query?host=web01&name=cpu.usage_pct&from=...&to=...&bucket=5m&agg=avg
    │
    ▼
JWTMiddleware
    │
    ▼
HandleQuery:
    parse from/to as RFC3339
    defaults: bucket=1m, agg=avg
    storage.QueryParams.Validate()
        - host, name required
        - name in allowlist
        - bucket in {1m,5m,15m,1h,1d}
        - agg in {avg,max,min}
        - from < to, range ≤ 30 days
    │
    ▼
storage.pgxMetricRepository.Query(ctx, params)
    │
    ├── allowlist interpolation: bucket → "5 minutes", agg → "avg"
    └── SELECT time_bucket('5 minutes', time) AS bucket,
               avg(value)
        FROM metrics
        WHERE host=$1 AND name=$2 AND time>=$3 AND time<=$4
        GROUP BY bucket ORDER BY bucket DESC
            │
            ▼
[]storage.QueryPoint{Time, Value}
    │
    ▼
JSON response: {host, name, bucket, agg, points:[{time,value},...]}
```

---

## 4. Package Structure

```
github.com/kamerrezz/theminidog
├── cmd/
│   ├── agent/         — Agent binary entrypoint. Wires config → collectors → sender → Agent.Run().
│   │                    Mints JWT on startup if AGENT_TOKEN is set.
│   └── server/        — Server binary entrypoint. Runs migrations, builds pgxpool,
│                        wires router, starts HTTP server with graceful shutdown.
│
├── internal/
│   ├── model/         — Shared domain types: Metric, MetricBatch, Validate().
│   │                    The only package imported by both agent and server internals.
│   │                    Zero external dependencies.
│   │
│   ├── config/        — Environment-variable configuration loaders.
│   │   ├── agent.go   — LoadAgent(): reads SERVER_URL, AGENT_TOKEN, COLLECT_INTERVAL, etc.
│   │   └── server.go  — LoadServerConfig(): reads DATABASE_URL, AGENT_TOKEN, LISTEN_ADDR, etc.
│   │                    Both fail-fast on required fields; use safe defaults for optional ones.
│   │
│   ├── agent/
│   │   ├── agent.go   — Agent struct: orchestrates collect loop + sender loop via two goroutines.
│   │   │                Uses local interfaces (registry, senderIface) to avoid import coupling.
│   │   ├── collector/ — Collector interface + Registry + four concrete collectors.
│   │   │   ├── collector.go  — Collector interface, Registry, CollectAll (errors are non-fatal per collector).
│   │   │   ├── cpu.go        — CPUCollector: aggregate + per-core cpu.usage_pct.
│   │   │   ├── memory.go     — MemoryCollector: mem.used_pct, mem.used_bytes, mem.total_bytes.
│   │   │   ├── disk.go       — DiskCollector: per-mount disk.used_pct/bytes/total; skips failed mounts.
│   │   │   └── network.go    — NetworkCollector: delta net.bytes_in/out per iface; first tick seeds state.
│   │   └── sender/
│   │       └── sender.go     — HTTPSender: JSON POST with exponential backoff + jitter. WithToken() option.
│   │
│   └── server/
│       ├── server.go  — Server struct: wraps http.Server + pgxpool for lifecycle management.
│       ├── api/
│       │   ├── router.go     — NewRouter(): chi router, middleware chain, route registration.
│       │   ├── middleware.go — JWTMiddleware: validates HS256 Bearer tokens, blocks alg=none.
│       │   ├── metrics.go    — HandleIngest + HandleQuery handlers.
│       │   ├── health.go     — HandleHealthz (liveness) + HandleReadyz (DB ping readiness).
│       │   └── errors.go     — writeError helper: consistent JSON error responses.
│       └── storage/
│           └── metrics.go    — pgxMetricRepository: Insert (pgx.Batch) + Query (time_bucket allowlist).
│
└── migrations/
    └── 001_create_metrics.up.sql  — CREATE EXTENSION timescaledb, CREATE TABLE metrics,
                                     create_hypertable, CREATE INDEX.
```

**Isolation rationale:** `internal/model` is the only cross-boundary package. Keeping it dependency-free prevents transitive dependency problems. `internal/agent` and `internal/server` are completely isolated — neither imports the other. The `cmd/` packages are the only place that wires everything together.

---

## 5. Dependency Graph

```
cmd/agent
    ├── internal/config
    ├── internal/agent          (agent.go)
    │       ├── internal/model
    │       └── [local interfaces: registry, senderIface]
    ├── internal/agent/collector
    │       └── internal/model
    │       └── gopsutil/v4
    └── internal/agent/sender
            └── internal/model

cmd/server
    ├── internal/config
    ├── internal/server         (server.go)
    ├── internal/server/api
    │       ├── internal/model
    │       ├── internal/server/storage
    │       ├── chi/v5
    │       └── golang-jwt/jwt/v5
    └── internal/server/storage
            ├── internal/model
            └── pgx/v5

internal/model   ← no external or internal imports (only stdlib)
```

**No cycles.** `internal/agent` uses local interfaces to reference `collector.Registry` and `sender.HTTPSender` without importing those packages directly. This keeps the dependency graph a strict DAG.

---

## 6. Storage Design

### Why TimescaleDB

PostgreSQL alone can handle time-series workloads at small scale, but its planner does not partition data by time automatically. TimescaleDB's hypertable transparently partitions the `metrics` table into 1-day chunks. This means:

- Range scans (`WHERE time >= $3 AND time <= $4`) only touch the relevant chunks
- `time_bucket()` is a native function, not a workaround
- Chunk compression can be added later without schema changes

### Schema

```sql
CREATE TABLE metrics (
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

CREATE INDEX idx_metrics_host_name_time ON metrics (host, name, time DESC);
```

### Why a narrow model

Rather than separate tables per metric type (`cpu_metrics`, `disk_metrics`, etc.), MiniObserv uses a single `(time, host, name, value, labels)` row per measurement. Trade-offs:

| Concern | Wide (separate tables) | Narrow (single table) |
|---|---|---|
| Schema changes when adding a metric | ALTER TABLE | Insert new name into allowlist |
| Query complexity for a single metric | Simple | Simple (`WHERE name = $1`) |
| Cross-metric queries | JOIN required | Single scan with `WHERE name IN (...)` |
| Storage overhead | Lower (typed columns) | Slightly higher (TEXT for name) |

The labels JSONB column carries per-metric context (e.g., `{"core": "0"}`, `{"mount": "/"}`, `{"iface": "eth0"}`) without polluting the table with sparse nullable columns.

### Why no ORM

pgx/v5 is used directly. ORMs add abstraction over PostgreSQL features that are first-class in pgx: `pgx.Batch` for bulk inserts and TimescaleDB functions like `time_bucket`. An ORM would fight both of those. Raw SQL keeps the full TimescaleDB feature set accessible.

---

## 7. Authentication Flow

MiniObserv uses HMAC-SHA256 (HS256) JWT. Both sides share a single secret (`AGENT_TOKEN`).

```
Agent startup
    │
    ▼
mintAgentToken(secret string) → jwt.RegisteredClaims{
    Issuer:    "miniobserv-agent",
    ExpiresAt: now + 24h,
    IssuedAt:  now,
}
    │
    ▼  jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
    │
    ▼  stored in HTTPSender.token
    │
Every HTTP request:
    Authorization: Bearer <token>

Server — JWTMiddleware:
    │
    ▼  jwt.ParseWithClaims(tokenStr, &jwt.RegisteredClaims{},
           func(t *jwt.Token) (any, error) { return []byte(secret), nil },
           jwt.WithValidMethods([]string{"HS256"}),   ← blocks alg=none and RS256 substitution
       )
    │
    ▼  401 Unauthorized on any error; next handler on success
```

**Key points:**
- The agent re-mints a fresh 24-hour token on each startup. There is no token refresh during a run.
- `WithValidMethods` is mandatory. Without it, an attacker can set `alg: none` in the token header and bypass signature validation.
- The secret must be at least 16 characters (enforced by `LoadServerConfig`).

---

## 8. Metric Naming Convention

MiniObserv defines exactly 9 canonical metric names. Both `internal/model` and `internal/server/storage` maintain an allowlist; they must be kept in sync when adding metrics.

| Name | Unit | Labels | Emitted by |
|---|---|---|---|
| `cpu.usage_pct` | % (0–100) | `core=total` or `core=<n>` | CPUCollector |
| `mem.used_pct` | % (0–100) | — | MemoryCollector |
| `mem.used_bytes` | bytes | — | MemoryCollector |
| `mem.total_bytes` | bytes | — | MemoryCollector |
| `disk.used_pct` | % (0–100) | `mount=<path>` | DiskCollector |
| `disk.used_bytes` | bytes | `mount=<path>` | DiskCollector |
| `disk.total_bytes` | bytes | `mount=<path>` | DiskCollector |
| `net.bytes_in` | bytes (delta) | `iface=<name>` | NetworkCollector |
| `net.bytes_out` | bytes (delta) | `iface=<name>` | NetworkCollector |

**Label key reference:**

- `core` — logical CPU index or `"total"` for the aggregate
- `mount` — mount point path, e.g. `"/"`, `"/data"`
- `iface` — network interface name, e.g. `"eth0"`, `"ens3"` (loopback `lo` always excluded)

---

## 9. Network Metric Delta Semantics

`net.bytes_in` and `net.bytes_out` are **cumulative OS counters** expressed as **per-tick deltas**.

The OS kernel maintains a running total of bytes received and sent per interface since boot. MiniObserv subtracts the previous snapshot from the current one to produce the bytes transferred in the last collection interval.

**First-tick behavior:** On the very first `Collect()` call, the NetworkCollector seeds its `prev` snapshot and returns `nil` (no metrics). This means:

- The first batch from any agent startup will contain CPU, memory, and disk metrics, but **no network metrics**.
- Network metrics appear starting from the **second** tick.

**Dashboard implication:** A gap at the beginning of a host's timeline in the `net.*` series is normal, not a data loss. When building dashboards, treat the first data point of a network series as the start of measurement, not an anomaly.

**Counter wrap / reset:** If the OS counter resets (reboot, counter overflow), the delta would be negative. MiniObserv clamps negative deltas to zero rather than emitting invalid values. A sudden drop to zero in the network series may indicate an agent restart or interface reset.

---

## 10. Configuration Reference

### Agent (`cmd/agent`)

| Environment Variable | Required | Default | Description |
|---|---|---|---|
| `SERVER_URL` | **Yes** | — | Full HTTP/HTTPS URL of the server, e.g. `http://server:8080`. Must be `http://` or `https://`. |
| `AGENT_TOKEN` | No | `""` | Shared HS256 secret. If empty, no `Authorization` header is sent. Must match the server's `AGENT_TOKEN`. |
| `AGENT_HOST` | No | `os.Hostname()` | Hostname label attached to every metric. Override when the OS hostname is not meaningful (e.g., in containers). |
| `COLLECT_INTERVAL` | No | `10s` | Collection tick duration. Must be a valid Go duration between `1s` and `300s`. Values outside this range fall back to `10s` silently. |
| `LOG_LEVEL` | No | `info` | Log verbosity. Valid values: `debug`, `info`, `warn`, `error`. |
| `LOG_PATHS` | No | `""` | Comma-separated log file paths (reserved for future log collection; unused in current release). |

The agent does not expose `DISK_MOUNTS` or `NET_IFACES` via environment variables in the current release. Those fields exist in `AgentConfig` but are not yet wired to env vars; all mounts and non-loopback interfaces are collected by default.

### Server (`cmd/server`)

| Environment Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | **Yes** | — | PostgreSQL DSN, e.g. `postgres://user:pass@db:5432/miniobserv`. Must use `postgres://` or `postgresql://` scheme. |
| `AGENT_TOKEN` | **Yes** | — | Shared HS256 secret used to validate agent JWTs. Minimum 16 characters. Must match the agent's `AGENT_TOKEN`. |
| `LISTEN_ADDR` | No | `:8080` | TCP address the HTTP server binds to. |
| `MIGRATIONS_PATH` | No | `./migrations` | Path to the directory containing `.sql` migration files. In Docker, set to `/app/migrations`. |
| `LOG_LEVEL` | No | `info` | Log verbosity. Valid values: `debug`, `warn`, `error`. |
| `REQUEST_TIMEOUT` | No | `10s` | Per-request timeout. Clamped between `1s` and `120s`. |
| `SHUTDOWN_TIMEOUT` | No | `5s` | Graceful shutdown drain period. Clamped between `1s` and `30s`. |

### Example: minimal docker-compose env block

```yaml
# agent
environment:
  SERVER_URL: http://server:8080
  AGENT_TOKEN: "change-me-at-least-16-chars"
  AGENT_HOST: web01

# server
environment:
  DATABASE_URL: postgres://miniobserv:secret@db:5432/miniobserv
  AGENT_TOKEN: "change-me-at-least-16-chars"
  LISTEN_ADDR: ":8080"
  MIGRATIONS_PATH: /app/migrations
```
