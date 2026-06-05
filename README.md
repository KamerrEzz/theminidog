# MiniObserv

> A self-hosted, learning-grade observability platform built in Go 1.23+.
> Collect metrics and logs from your servers, store them in TimescaleDB,
> query them through a REST API, and monitor thresholds with a live dashboard.

Built as a **4-week deep dive** into how observability tools like Datadog work under the hood — no black boxes.

---

## What it does

```
  YOUR SERVERS                        MINIOBSERV SERVER              TIMESCALEDB
  ─────────────────────               ─────────────────────          ─────────────
  ┌─────────────┐  JWT/HTTP           ┌──────────────────┐
  │    AGENT    │ ─────────────────►  │  chi router      │  ──────►  metrics
  │             │  POST /metrics      │  JWT middleware   │           (hypertable)
  │  cpu        │  POST /logs         │  ingest handlers │
  │  memory     │                     │                  │  ──────►  logs
  │  disk       │  ◄──────────────── │  query API        │           (BIGSERIAL)
  │  network    │    202 Accepted     │                  │
  │  log files  │                     │  alerting ticker │
  └─────────────┘                     │  dashboard       │
                                      └──────────────────┘
                                              │
                                         http://localhost:8080
```

**Agent** runs on each host, collects 9 system metrics every 10s, tails log files, and ships everything via authenticated HTTP batches with exponential backoff.

**Server** validates, bulk-inserts via `pgx.Batch`, exposes a time-bucket query API, evaluates threshold alert rules every 30s, and serves a live dashboard.

---

## Features

| | Feature | Tech |
|---|---|---|
| ✅ | System metrics (CPU, RAM, Disk, Network) | gopsutil/v4 |
| ✅ | Log file tailing with rotation detection | fsnotify/v1 |
| ✅ | TimescaleDB hypertable for time-series metrics | pgx/v5 + TimescaleDB |
| ✅ | Keyset-paginated log query | PostgreSQL BIGSERIAL |
| ✅ | JWT authentication (HS256, shared secret) | golang-jwt/v5 |
| ✅ | Auto-migrations on startup | golang-migrate/v4 |
| ✅ | Threshold alerting (>/<, any metric, any host) | in-memory evaluator |
| ✅ | Live dashboard with SVG sparklines | html/template + //go:embed |
| ✅ | Docker Compose full stack | multi-stage builds |
| ✅ | TypeScript SDK (zero runtime deps) | node:crypto for JWT |
| ✅ | 213 unit tests, strict TDD | go test ./... |

---

## Quick Start

```bash
git clone https://github.com/KamerrEzz/theminidog.git
cd theminidog/deployments

# Optional: change AGENT_TOKEN to a strong secret
docker compose up --build
```

Open **http://localhost:8080** — the live dashboard appears as soon as the agent starts collecting.

```bash
# Verify everything is running
curl http://localhost:8080/healthz   # → ok
curl http://localhost:8080/readyz    # → ok  (DB connected)
curl http://localhost:8080/api/v1/alerts  # → {"alerts":[]}
```

### Demo with load generator

```bash
cd example/06-demo-app
docker compose up --build
# → http://localhost:8080  (MiniObserv dashboard)
# → http://localhost:9000  (Task API being monitored)
```

The demo spins up a Task REST API + load generator that pushes requests every 2s.
MiniObserv tails the app's log file and fires a `mem.used_pct > 8` alert automatically.

---

## Dashboard

Live-updating every 5 seconds. No page reload.

- Dark theme with SVG sparkline charts per metric
- Trend indicators (↑ rising / ↓ falling / → stable)
- Animated FIRING/OK alert badges
- Structured log stream from tailed files
- Human-readable metric names (CPU Usage, Memory Used, etc.)

```
┌─ MiniObserv ──────────────────── ● live  🔴 1 firing ─┐
├─ HOSTS ──┬─────────────────────────────────────────────┤
│          │  ⚠ Memory Used > 8 | actual: 10.36%         │
│ ● a26af… │                                             │
│          │  ┌────────────────┐  ┌────────────────┐     │
│          │  │ CPU Usage      │  │ Memory Used    │     │
│          │  │   0.70%        │  │   10.19%       │     │
│          │  │ → stable       │  │ ↑ rising       │     │
│          │  │ ▁▂▁▁▂▁▂▁▁     │  │ ▃▄▅▅▄▅▅▄▅▅     │     │
│          │  └────────────────┘  └────────────────┘     │
│          │                                             │
│          │  Logs (20)                                  │
│          │  16:42:31  INFO  GET /tasks → 200 (0ms)     │
│          │  16:42:31  INFO  POST /tasks → 201 (0ms)    │
└──────────┴─────────────────────────────────────────────┘
```

---

## Alert Rules

Set `ALERT_RULES` as a JSON array:

```json
[
  {"host":"*","name":"cpu.usage_pct","op":">","threshold":80,"for":"5m"},
  {"host":"web-01","name":"mem.used_pct","op":">","threshold":85,"for":"10m"}
]
```

`host: "*"` evaluates the rule against all known hosts. Alerts log via `slog.Error` when firing and `slog.Info` when resolved.

---

## API

```
# Public (no auth)
GET  /                              Live dashboard
GET  /healthz                       Liveness probe
GET  /readyz                        Readiness + DB ping
GET  /api/v1/alerts                 Current alert states
GET  /api/v1/dashboard/metrics      Metrics for dashboard JS
GET  /api/v1/dashboard/logs         Logs for dashboard JS

# JWT-authenticated
POST /api/v1/metrics                Ingest metric batch
GET  /api/v1/metrics/query          Query time-bucketed series
POST /api/v1/logs                   Ingest log batch
GET  /api/v1/logs/query             Filtered + paginated log search
```

Full spec → [docs/api-reference.md](docs/api-reference.md)

---

## TypeScript SDK

```bash
npm install @kamerrezz/miniobserv
```

```typescript
import { MiniObservClient } from '@kamerrezz/miniobserv'

const client = new MiniObservClient({
  baseUrl: 'http://localhost:8080',
  agentToken: process.env.AGENT_TOKEN!,
  defaultHost: 'my-app',
})

// Push a metric
await client.pushMetric('cpu.usage_pct', 42.5, { core: 'total' })

// Query last hour
const result = await client.queryMetrics({
  host: 'my-app',
  name: 'cpu.usage_pct',
  from: new Date(Date.now() - 3600_000),
  to: new Date(),
  bucket: '5m',
  agg: 'avg',
})
```

Auto-mints 24h HS256 JWTs from your `AGENT_TOKEN` secret. Zero runtime dependencies — uses `node:crypto`.

---

## Configuration

### Agent

| Variable | Default | Description |
|---|---|---|
| `SERVER_URL` | required | MiniObserv server base URL |
| `AGENT_TOKEN` | required | Shared HS256 secret (min 16 chars) |
| `COLLECT_INTERVAL` | `10s` | Metric collection frequency |
| `AGENT_HOST` | OS hostname | Host label for all metrics |
| `LOG_PATHS` | — | Comma-separated log files to tail |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

### Server

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | required | `postgres://` DSN |
| `AGENT_TOKEN` | required | Same secret as agent |
| `LISTEN_ADDR` | `:8080` | Bind address |
| `ALERT_RULES` | — | JSON array of threshold rules |
| `DASHBOARD_ENABLED` | `true` | Set `false` to disable `GET /` |
| `MIGRATIONS_PATH` | `./migrations` | SQL migration files path |
| `LOG_LEVEL` | `info` | Server log verbosity |

---

## Project Structure

```
theminidog/
├── cmd/
│   ├── agent/              # Agent binary
│   └── server/             # Server binary
├── internal/
│   ├── agent/
│   │   ├── collector/      # CPU, memory, disk, network (statFn injection)
│   │   ├── logtail/        # fsnotify file tailing + ParseLevel
│   │   └── sender/         # HTTP batch client, exponential backoff
│   ├── server/
│   │   ├── alerting/       # Threshold evaluator, sync.RWMutex state
│   │   ├── api/            # chi router, JWT middleware, handlers
│   │   ├── dashboard/      # html/template + //go:embed
│   │   └── storage/        # pgx MetricRepository + LogRepository
│   ├── config/             # Env-driven config (fail-fast on required)
│   └── model/              # Metric, MetricBatch, LogEntry, LogBatch
├── migrations/             # golang-migrate SQL files
├── sdk/js/                 # @kamerrezz/miniobserv TypeScript SDK
├── example/
│   ├── 01-curl-quickstart/ # Full API lifecycle in shell
│   ├── 02-node-push-metrics/
│   ├── 03-node-query-dashboard/
│   ├── 04-go-http-client/
│   ├── 05-web-dashboard/
│   └── 06-demo-app/        # Task API + load gen + full MiniObserv stack
├── docs/
│   ├── architecture.md
│   ├── decisions.md        # 14 Architecture Decision Records
│   ├── getting-started.md
│   ├── api-reference.md
│   ├── internals.md        # Guide for contributors
│   ├── observability-concepts.md
│   └── es/                 # All docs in Spanish
├── Dockerfile.agent
├── Dockerfile.server
└── deployments/
    └── docker-compose.yml
```

---

## Documentation

| Doc | Description |
|---|---|
| [Getting Started](docs/getting-started.md) | Prerequisites, build, Docker Compose, troubleshooting |
| [API Reference](docs/api-reference.md) | Full endpoint spec with curl examples |
| [Architecture](docs/architecture.md) | Component diagrams, data flow, storage design |
| [Decisions](docs/decisions.md) | 14 ADRs explaining every non-obvious choice |
| [Internals](docs/internals.md) | How to add collectors, endpoints, and tests |
| [Concepts](docs/observability-concepts.md) | Observability from zero — for newcomers |
| [Español](docs/es/) | Toda la documentación en español |

---

## Development

```bash
# Run all tests
go test ./...          # 213 tests

# Build both binaries
make build-agent
make build-server

# Run with stub server (no DB needed)
go run ./cmd/stubserver &
SERVER_URL=http://localhost:8080 go run ./cmd/agent
```

---

## How it was built

This project was built **spec-first** using [Spec-Driven Development (SDD)](https://gentle-ai.com/sdd):

```
explore → propose → spec → design → tasks → apply → verify
```

Each week delivered a vertical slice. Every architectural decision is documented as an ADR in [docs/decisions.md](docs/decisions.md).

---

## License

MIT
