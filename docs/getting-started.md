# Getting Started with MiniObserv

This guide walks you through building, configuring, and running MiniObserv from source — both locally and with Docker Compose.

---

## Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| Go | 1.23+ | [go.dev/dl](https://go.dev/dl/) |
| Docker + Docker Compose | latest stable | [docs.docker.com](https://docs.docker.com/get-docker/) |
| make | any | `brew install make` / `apt install make` |

> **Windows PATH note:** if `go` is not found in your terminal, add it to your session:
> ```powershell
> $env:PATH += ";C:\Program Files\Go\bin"
> ```

---

## Clone & Build

```bash
git clone https://github.com/kamerrezz/theminidog.git
cd theminidog

# Build both binaries into bin/
make build-agent    # → bin/agent
make build-server   # → bin/server
```

You can also run without building:

```bash
go run ./cmd/agent
go run ./cmd/server
```

---

## Configuration

All settings are read from environment variables. Neither binary reads a config file.

### Agent environment variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SERVER_URL` | yes | — | HTTP/HTTPS base URL of the server. Must include scheme. |
| `AGENT_TOKEN` | yes (with auth server) | — | Shared HS256 secret. Min 16 chars. Must match the server. |
| `AGENT_HOST` | no | OS hostname | Label attached to every metric. |
| `COLLECT_INTERVAL` | no | `10s` | Collection frequency. Accepts Go durations (`1s`–`300s`). |
| `LOG_LEVEL` | no | `info` | One of: `debug`, `info`, `warn`, `error`. |
| `LOG_PATHS` | no | — | Comma-separated file paths to tail (Week 3 feature). |

### Server environment variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | yes | — | PostgreSQL DSN. **Must** use `postgres://` scheme. |
| `AGENT_TOKEN` | yes | — | Shared HS256 secret. Min 16 chars. Must match the agent. |
| `LISTEN_ADDR` | no | `:8080` | TCP address to bind. |
| `MIGRATIONS_PATH` | no | `./migrations` | Path to SQL migration files. |
| `LOG_LEVEL` | no | `info` | One of: `debug`, `info`, `warn`, `error`. |
| `REQUEST_TIMEOUT` | no | `10s` | Per-request timeout. Range: `1s`–`120s`. |
| `SHUTDOWN_TIMEOUT` | no | `5s` | Graceful shutdown window. Range: `1s`–`30s`. |
| `ALERT_NOTIFICATIONS` | no | — | JSON array of webhook objects. See [Notifications](#notifications) below. |
| `HOST_STALE_AFTER` | no | `20s` | Duration after which a silent host is marked stale. Accepts Go duration strings. |
| `HOST_DOWN_AFTER` | no | `50s` | Duration after which a silent host is marked down and a `host.down` webhook fires. |

---

## Notifications

MiniObserv can POST a JSON payload to one or more HTTP webhooks whenever a threshold alert fires or resolves — and when a host goes silent beyond `HOST_DOWN_AFTER`.

Set `ALERT_NOTIFICATIONS` to a JSON array of webhook objects:

```bash
ALERT_NOTIFICATIONS='[{"type":"webhook","url":"https://hooks.slack.com/services/YOUR/WEBHOOK/URL"}]'
```

Each webhook receives a payload like:

```json
{"event":"firing","rule":{...},"value":10.36,"fired_at":"2026-06-05T16:42:23Z"}
```

- `event` is `"firing"` when the threshold is crossed, `"resolved"` when it recovers
- Delivery is fire-and-forget with a 5-second timeout — no retries in v1
- Works with any HTTP webhook: Slack, Discord, Teams, PagerDuty, or a custom endpoint

**Multiple destinations:**

```bash
ALERT_NOTIFICATIONS='[
  {"type":"webhook","url":"https://hooks.slack.com/services/YOUR/WEBHOOK/URL"},
  {"type":"webhook","url":"https://discord.com/api/webhooks/YOUR/WEBHOOK"}
]'
```

---

## Grafana

MiniObserv exposes `GET /metrics` in Prometheus text format. To add Grafana:

```bash
cd deployments
docker compose -f docker-compose.yml -f grafana/docker-compose.yml up
```

Full guide → [Grafana Integration](grafana.md)

---

## Generating an AGENT_TOKEN

The token is a shared secret used by both the agent and the server. It must be at least 16 characters. Use a cryptographically random value in production.

**Generate a strong 32-character secret:**

```bash
# Linux / macOS
openssl rand -hex 32

# Any platform with Go installed
go run -v - <<'EOF'
package main
import (
    "crypto/rand"
    "encoding/hex"
    "fmt"
)
func main() {
    b := make([]byte, 32)
    rand.Read(b)
    fmt.Println(hex.EncodeToString(b))
}
EOF
```

Set the same value in both the agent and server environments.

**Mint a JWT manually** (useful for curl testing):

```bash
# Using jwt-cli  (npm install -g jwt-cli)
jwt sign --secret "YOUR_SECRET_HERE" --alg HS256 \
  '{"iss":"miniobserv-agent","exp":'$(( $(date +%s) + 86400 ))'}'
```

Or with any JWT HS256 library in your preferred language.

---

## Running with Docker Compose

The `deployments/docker-compose.yml` starts TimescaleDB, the server, and the agent together. The server waits for the DB health check before starting; the agent waits for the server health check.

```bash
# 1. Set a real secret (replace the placeholder)
#    Edit deployments/docker-compose.yml and change:
#      AGENT_TOKEN: "change-me-use-a-real-secret-min-16ch"
#    to a value from the section above.

# 2. Start the full stack
cd deployments
docker compose up --build
```

What to expect:

```
timescaledb  | database system is ready to accept connections
server       | {"level":"INFO","msg":"running migrations"}
server       | {"level":"INFO","msg":"server listening","addr":":8080"}
agent        | {"level":"INFO","msg":"agent starting","host":"...","interval":"10s"}
agent        | {"level":"INFO","msg":"batch sent","ingested":9}
```

- TimescaleDB starts on its internal port (not exposed to the host by default).
- The server is available at `http://localhost:8080`.
- The agent collects every 10 seconds and pushes to the server inside the Docker network.

To stop the stack:

```bash
docker compose down          # stop and remove containers
docker compose down -v       # also remove the TimescaleDB volume (clears all data)
```

---

## Running the Agent Standalone

You can run the agent binary against any server — local or remote.

```bash
export SERVER_URL=http://localhost:8080
export AGENT_TOKEN=YOUR_SECRET_HERE
export COLLECT_INTERVAL=10s
export LOG_LEVEL=debug

./bin/agent
# or: go run ./cmd/agent
```

The agent mints a 24h JWT from `AGENT_TOKEN` on startup and uses it for all requests. If the token expires while running, restart the agent.

---

## Verifying It Works

After the stack is running (or the agent + server are both started), run these checks.

**Liveness probe:**

```bash
curl http://localhost:8080/healthz
# Expected: 200 OK  "ok"
```

**Readiness probe (also pings the DB):**

```bash
curl http://localhost:8080/readyz
# Expected: 200 OK  "ok"
# If the DB is not reachable: 503 Service Unavailable
```

**Wait ~30 seconds** for the agent to collect and push at least two ticks, then query metrics:

```bash
# Generate a JWT first
TOKEN=$(jwt sign --secret "YOUR_SECRET_HERE" --alg HS256 \
  '{"iss":"miniobserv-agent","exp":'$(( $(date +%s) + 86400 ))'}'  )

# Query the last 5 minutes of CPU usage
curl -s -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/metrics/query?host=YOUR_HOSTNAME&name=cpu.usage_pct&from=$(date -u -d '-5 minutes' +%Y-%m-%dT%H:%M:%SZ)&to=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  | jq .
```

A successful response looks like:

```json
{
  "host": "YOUR_HOSTNAME",
  "name": "cpu.usage_pct",
  "bucket": "1m",
  "agg": "avg",
  "points": [
    {"time": "2026-06-05T10:00:00Z", "value": 14.2},
    {"time": "2026-06-05T10:01:00Z", "value": 18.7}
  ]
}
```

If `points` is empty but the request succeeds, wait another collection interval and try again.

---

## Stopping Gracefully

Both the agent and the server handle `SIGINT` and `SIGTERM`.

- **Agent:** on signal, the collection loop stops and the process exits after the current tick completes. In-flight batches are not retried after shutdown begins.
- **Server:** on signal, new connections are rejected and in-flight requests are given `SHUTDOWN_TIMEOUT` (default 5s) to complete. The DB connection pool is closed cleanly.

```bash
# Ctrl+C in the terminal, or:
kill -SIGTERM <pid>
```

With Docker Compose, `docker compose down` sends SIGTERM to all containers.

---

## Troubleshooting

### `DATABASE_URL must be a valid postgres:// URL`

The server requires the DSN to start with `postgres://` or `postgresql://`. The DSN scheme used by some tools (`pgx://`, `pgx5://`) is **not** valid here — MiniObserv rewrites the scheme internally for the migration driver.

Correct:
```
DATABASE_URL=postgres://minidog:minidog@localhost:5432/miniobserv?sslmode=disable
```

### `AGENT_TOKEN must be at least 16 characters`

The server rejects tokens shorter than 16 characters at startup. Generate a new secret with the command in the [Generating an AGENT_TOKEN](#generating-an-agent_token) section.

### `SERVER_URL must be a valid http/https URL`

The agent requires the URL to start with `http://` or `https://`. A bare hostname or IP is not valid.

Correct:
```
SERVER_URL=http://localhost:8080
```

### Network metrics are empty on the first tick

`net.bytes_in` and `net.bytes_out` are **deltas**: the agent records the cumulative byte counters at startup and emits the difference on the next tick. There are no network metrics on the very first collection. This is expected behavior — wait for the second tick.

### `401 unauthorized` when querying the API

- Confirm `AGENT_TOKEN` matches between agent and server.
- Check that the JWT has not expired (24h lifetime).
- Mint a fresh token with the jwt-cli command above.

### Server starts but `readyz` returns 503

The server can start and accept connections before the DB is ready (this can happen if you run the server binary outside of Docker Compose without a healthy DB). Wait for TimescaleDB to finish initializing, or check that `DATABASE_URL` points to the correct host and port.

### Port 8080 already in use

Change `LISTEN_ADDR` on the server and `SERVER_URL` on the agent to a different port, or stop the process using 8080.
