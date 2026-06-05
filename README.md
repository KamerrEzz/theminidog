# MiniObserv

A self-hosted observability platform — a learning-grade mini Datadog — built in Go 1.23+.
Collects system metrics (CPU, RAM, Disk, Network) from hosts, ships them to a central server,
and persists them in TimescaleDB for querying and alerting.

## Documentation

- [Getting Started](docs/getting-started.md) — prerequisites, build, configuration, Docker Compose, troubleshooting
- [API Reference](docs/api-reference.md) — full endpoint spec, authentication, metric names, integration examples

## Scope

| Feature | Status |
|---------|--------|
| Agent — system metrics collection | ✅ Week 1 |
| Server — metrics ingestion + TimescaleDB | ✅ Week 2 |
| Logs — agent tailing + server ingestion | ⏳ Week 3 |
| Dashboard + threshold alerting | ⏳ Week 4 |

## Architecture

```
  HOST(s)                            SERVER                      STORAGE
┌─────────────────────┐  HTTP/JSON  ┌────────────────────┐   ┌─────────────────┐
│       AGENT         │  (batched)  │     SERVER         │   │ PostgreSQL 17   │
│                     │  ────────►  │                    │   │ + TimescaleDB   │
│ collector/          │  /metrics   │ api/   (chi)       ├──►│                 │
│   cpu, mem,         │  /logs      │ storage/ (pgx)     │   │ metrics (hyper) │
│   disk, network     │             │ alerting/          │   │ logs   (table)  │
│                     │             │ dashboard/         │   └─────────────────┘
│ sender/ (backoff)   │  ◄────────  │                    │
└─────────────────────┘   202 ack   └────────────────────┘
```

## Project Structure

```
miniobserv/
├── cmd/
│   ├── agent/          # Agent binary entrypoint
│   ├── server/         # Server binary entrypoint
│   └── stubserver/     # Stub server for local testing (no DB)
├── internal/
│   ├── agent/
│   │   ├── collector/  # CPU, memory, disk, network collectors
│   │   ├── sender/     # HTTP batch client with exponential backoff
│   │   └── agent.go    # Collection loop + sender coordination
│   ├── server/
│   │   ├── api/        # HTTP router, handlers, JWT middleware
│   │   └── storage/    # TimescaleDB repository (pgx)
│   ├── config/         # Environment-variable driven config
│   └── model/          # Shared types: Metric, MetricBatch
├── migrations/         # SQL migration files (golang-migrate)
├── deployments/
│   └── docker-compose.yml
├── Dockerfile.agent
├── Dockerfile.server
├── Dockerfile.stubserver
├── Makefile
└── .env.example
```

## Prerequisites

- [Go 1.23+](https://go.dev/dl/)
- [Docker + Docker Compose](https://docs.docker.com/get-docker/)

> **Windows PATH note:** if `go` is not found in your terminal, add
> `C:\Program Files\Go\bin` to your session: `$env:PATH += ";C:\Program Files\Go\bin"`

## Quick Start

### Full stack — server + TimescaleDB + agent (Docker Compose)

```bash
# 1. Set a real secret in deployments/docker-compose.yml
#    Replace "change-me-use-a-real-secret-min-16ch" with a strong value:
#      openssl rand -hex 32

# 2. Start everything
cd deployments
docker compose up --build
```

The server is available at `http://localhost:8080`. The agent collects every 10 seconds
and pushes to the server inside the Docker network. Metrics appear in TimescaleDB.

```bash
# Verify the server is up
curl http://localhost:8080/healthz   # → ok
curl http://localhost:8080/readyz    # → ok (DB is reachable)
```

### Agent-only against a running server

```bash
export SERVER_URL=http://localhost:8080
export AGENT_TOKEN=YOUR_SECRET_HERE
export COLLECT_INTERVAL=10s
go run ./cmd/agent
```

### Development mode — stub server (no DB)

```bash
# Terminal 1
go run ./cmd/stubserver

# Terminal 2
export SERVER_URL=http://localhost:8080
go run ./cmd/agent
```

## API Usage

All API endpoints require a JWT. Generate one from the shared secret:

```bash
# Install jwt-cli once
npm install -g jwt-cli

TOKEN=$(jwt sign --secret "YOUR_SECRET_HERE" --alg HS256 \
  '{"iss":"miniobserv-agent","exp":'$(( $(date +%s) + 86400 ))'}'  )
```

**Push a metric manually:**

```bash
curl -s -X POST http://localhost:8080/api/v1/metrics \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "host": "web-01",
    "metrics": [{
      "time":   "2026-06-05T10:00:00Z",
      "host":   "web-01",
      "name":   "cpu.usage_pct",
      "value":  42.5,
      "labels": {"core": "total"}
    }]
  }'
# → {"ingested":1}
```

**Query metrics:**

```bash
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/metrics/query?host=web-01&name=cpu.usage_pct&from=2026-06-05T09:00:00Z&to=2026-06-05T10:00:00Z&bucket=5m&agg=avg" \
  | jq .
```

See [docs/api-reference.md](docs/api-reference.md) for the full API contract.

## Configuration

All settings are loaded from environment variables.

### Agent

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_URL` | *(required)* | HTTP/HTTPS base URL of the server |
| `AGENT_TOKEN` | *(required with server)* | Shared HS256 secret (min 16 chars) |
| `COLLECT_INTERVAL` | `10s` | How often to collect metrics (1s–300s) |
| `AGENT_HOST` | OS hostname | Label used for all metrics |
| `LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `LOG_PATHS` | *(empty)* | Comma-separated log files to tail (Week 3) |

### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | *(required)* | PostgreSQL DSN (`postgres://` scheme) |
| `AGENT_TOKEN` | *(required)* | Shared HS256 secret (min 16 chars) |
| `LISTEN_ADDR` | `:8080` | TCP address to bind |
| `MIGRATIONS_PATH` | `./migrations` | Path to SQL migration files |
| `LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `REQUEST_TIMEOUT` | `10s` | Per-request timeout (1s–120s) |
| `SHUTDOWN_TIMEOUT` | `5s` | Graceful shutdown window (1s–30s) |

## Development

```bash
make build-agent   # compile agent binary → bin/agent
make build-server  # compile server binary → bin/server
make test          # run all unit tests
make test-verbose  # verbose test output
make vet           # go vet
make tidy          # go mod tidy
make lint          # golangci-lint (requires golangci-lint installed)
```

Integration tests (require a running TimescaleDB) are guarded by the `integration` build tag:

```bash
go test -tags=integration ./...
```

## Metric Reference

The agent emits the following metric names:

| Name | Labels | Description |
|------|--------|-------------|
| `cpu.usage_pct` | `core=total\|0\|1…` | CPU usage percentage |
| `mem.used_pct` | — | Memory used percentage |
| `mem.used_bytes` | — | Memory used in bytes |
| `mem.total_bytes` | — | Total memory in bytes |
| `disk.used_pct` | `mount=/` | Disk used percentage per mount |
| `disk.used_bytes` | `mount=/` | Disk used bytes per mount |
| `disk.total_bytes` | `mount=/` | Disk total bytes per mount |
| `net.bytes_in` | `iface=eth0` | Network bytes received (delta per interval) |
| `net.bytes_out` | `iface=eth0` | Network bytes sent (delta per interval) |

> `net.*` metrics are **not emitted on the first collection tick** — the agent seeds
> cumulative counters on startup and emits deltas from the second tick onward.

## HTTP API

```
POST /api/v1/metrics        Ingest a MetricBatch
GET  /api/v1/metrics/query  Query time-bucketed metric series
POST /api/v1/logs           Ingest a LogBatch          (Week 3)
GET  /api/v1/logs/query     Filtered log search        (Week 3)
GET  /healthz               Liveness probe
GET  /readyz                Readiness probe (DB ping)
GET  /                      Dashboard                  (Week 4)
```

See [docs/api-reference.md](docs/api-reference.md) for the complete spec.

## Testing

Tests are unit-only for the agent (no real OS calls, no network, no DB).
All collectors inject their OS stat functions for full determinism.
The sender injects its sleep and HTTP functions for backoff testing.

```bash
make test
# ok  github.com/kamerrezz/theminidog/internal/agent           (95 tests)
# ok  github.com/kamerrezz/theminidog/internal/agent/collector
# ok  github.com/kamerrezz/theminidog/internal/agent/sender
# ok  github.com/kamerrezz/theminidog/internal/config
# ok  github.com/kamerrezz/theminidog/internal/model
```

## Roadmap

| Week | Theme | Key Components |
|------|-------|----------------|
| **1** ✅ | Agent metrics | `collector/*`, `sender`, `agent.go`, `cmd/agent` |
| **2** ✅ | Server + storage | `cmd/server`, `api/metrics`, `storage/metrics`, TimescaleDB migrations, JWT auth |
| **3** | Logs pipeline | `logtail`, `api/logs`, `storage/logs`, log rotation |
| **4** | Dashboard + alerting | `dashboard`, `alerting`, threshold rules |

### Week 2 — what shipped

- **Server binary** (`cmd/server`): HTTP server with chi router, JWT middleware (HS256), structured JSON logging with slog.
- **Ingestion endpoint** (`POST /api/v1/metrics`): validates batches, writes to TimescaleDB hypertable, returns ingested count.
- **Query endpoint** (`GET /api/v1/metrics/query`): time-bucketed aggregation (avg/max/min) with configurable bucket size (1m–1d).
- **Health probes** (`/healthz`, `/readyz`): liveness and readiness with DB ping.
- **TimescaleDB migrations**: automated on server startup via golang-migrate.
- **Docker Compose** updated to run the full stack (TimescaleDB + server + agent) with health-check ordering.
