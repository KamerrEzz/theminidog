# MiniObserv

A self-hosted observability platform — a learning-grade mini Datadog — built in Go 1.23+.
Collects system metrics (CPU, RAM, Disk, Network) and logs from hosts, ships them to a
central server, and persists them for querying and alerting.

## Scope

| Feature | Status |
|---------|--------|
| Agent — system metrics collection | ✅ Week 1 |
| Server — metrics ingestion + TimescaleDB | 🔄 Week 2 |
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
│   └── stubserver/     # Stub server for local testing (no DB)
├── internal/
│   ├── agent/
│   │   ├── collector/  # CPU, memory, disk, network collectors
│   │   ├── sender/     # HTTP batch client with exponential backoff
│   │   └── agent.go    # Collection loop + sender coordination
│   ├── config/         # Environment-variable driven config
│   └── model/          # Shared types: Metric, MetricBatch
├── migrations/         # SQL migration files (golang-migrate)
├── deployments/
│   └── docker-compose.yml
├── Dockerfile.agent
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

### Option A — Docker (agent + stub server, no DB needed)

```bash
cd deployments
docker compose up --build
```

The stub server listens on `localhost:8080` and logs every metric batch it receives.
The agent collects every 5 seconds and pushes batches to the stub server.

### Option B — Run locally

```bash
# Terminal 1: start the stub server
go run ./cmd/stubserver

# Terminal 2: start the agent
cp .env.example .env          # edit if needed
export SERVER_URL=http://localhost:8080
export COLLECT_INTERVAL=10s
export LOG_LEVEL=debug
go run ./cmd/agent
```

## Configuration

All settings are loaded from environment variables. Copy `.env.example` to `.env` and adjust.

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_URL` | *(required)* | HTTP/HTTPS base URL of the server |
| `COLLECT_INTERVAL` | `10s` | How often to collect metrics (1s–300s) |
| `AGENT_HOST` | OS hostname | Label used for all metrics |
| `LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `LOG_PATHS` | *(empty)* | Comma-separated log files to tail (Week 3) |

## Development

```bash
make test          # run all unit tests
make test-verbose  # verbose test output
make vet           # go vet
make build-agent   # compile agent binary to bin/
make tidy          # go mod tidy
make lint          # golangci-lint (requires golangci-lint installed)
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

> Available from Week 2 onward (server binary).

```
POST /api/v1/metrics        Ingest a MetricBatch
GET  /api/v1/metrics/query  Query time-bucketed metric series
POST /api/v1/logs           Ingest a LogBatch          (Week 3)
GET  /api/v1/logs/query     Filtered log search        (Week 3)
GET  /healthz               Liveness probe
GET  /readyz                Readiness probe (DB ping)
GET  /                      Dashboard                  (Week 4)
```

## Testing

Tests are unit-only in Week 1 (no real OS calls, no network, no DB).
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

Integration tests (hitting a real TimescaleDB) are introduced in Week 2
and guarded by the `integration` build tag:

```bash
go test -tags=integration ./...
```

## Roadmap

| Week | Theme | Key Components |
|------|-------|----------------|
| **1** ✅ | Agent metrics | `collector/*`, `sender`, `agent.go`, `cmd/agent` |
| **2** | Server + storage | `cmd/server`, `api/metrics`, `storage/metrics`, TimescaleDB migrations |
| **3** | Logs pipeline | `logtail`, `api/logs`, `storage/logs`, log rotation |
| **4** | Dashboard + alerting | `dashboard`, `alerting`, threshold rules |
