# 06 — MiniObserv End-to-End Demo

A fully self-contained demo that spins up the complete MiniObserv stack alongside a
real HTTP Task API and a load generator — all with a single command.

## Quick Start

```bash
cd example/06-demo-app
docker compose up --build
```

That's it. Docker Compose will build and start:

| Service       | Description                                     | Exposed         |
|---------------|-------------------------------------------------|-----------------|
| `timescaledb` | TimescaleDB (metrics persistence)               | internal only   |
| `server`      | MiniObserv server + dashboard                   | http://localhost:8080 |
| `agent`       | MiniObserv agent (collects metrics + tails logs)| internal only   |
| `demoapp`     | Task API (Go stdlib, in-memory)                 | http://localhost:9000 |
| `loadgen`     | Load generator (10 req/2s)                      | internal only   |

## What to Look At

### 1. MiniObserv Dashboard
Open **http://localhost:8080**

You will see the MiniObserv dashboard. After the first 5-second collection interval,
a host entry will appear (named after the agent container's hostname).

### 2. Task API (manual exploration)
Open **http://localhost:9000/tasks**

You can also interact with it directly:

```bash
# Create a task
curl -s -X POST http://localhost:9000/tasks \
  -H 'Content-Type: application/json' \
  -d '{"title":"my task"}' | jq

# List tasks
curl -s http://localhost:9000/tasks | jq

# Slow endpoint (100–500ms response)
curl -s http://localhost:9000/slow | jq

# CPU spike endpoint (fibonacci(35))
curl -s http://localhost:9000/cpu | jq
```

## What You Should See

### Metrics (~30 seconds after startup)
The dashboard shows a host entry (the agent's container ID) with live metrics:
- `cpu.usage_pct` — CPU percentage (the `/cpu` endpoint drives this up)
- `mem.used_pct` — Memory usage percentage
- `disk.used_pct` — Disk usage percentage

### Logs (immediately after startup)
The **Recent Logs** section shows structured JSON logs from the demo app:
```json
{"time":"...","level":"INFO","msg":"request","method":"GET","path":"/tasks","status":200,"duration_ms":3}
```

The agent tails `/var/log/demoapp/app.log` (shared via a Docker volume) and ships
log lines to the server in real time.

### CPU Alert (~60 seconds after startup)
The alert rule is configured with a very low threshold to ensure it fires during
the demo:

```
cpu.usage_pct > 5.0 for 30s
```

The `/cpu` endpoint runs `fibonacci(35)` on every request; the load generator
calls it once every 2 seconds. Within ~60 seconds you should see:

```
ALERT FIRING: cpu.usage_pct > 5.0 (host: <container-id>)
```

in the alerts section of the dashboard.

## Architecture

```
loadgen ──► demoapp:9000 ──► /var/log/demoapp/app.log
                                        │
                              agent (logtail) ──► server:8080
                              agent (metrics) ──►    │
                                                   dashboard
                                                   alerts
```

## Configuration

Key environment variables (already set in docker-compose.yml):

| Variable        | Service | Value                                |
|-----------------|---------|--------------------------------------|
| `AGENT_TOKEN`   | server  | `demo-secret-key-32chars-long!!`     |
| `AGENT_TOKEN`   | agent   | `demo-secret-key-32chars-long!!`     |
| `COLLECT_INTERVAL` | agent | `5s`                              |
| `LOG_PATHS`     | agent   | `/var/log/demoapp/app.log`           |
| `ALERT_RULES`   | server  | cpu.usage_pct > 5.0 for 30s         |
| `DEMO_URL`      | loadgen | `http://demoapp:9000`                |

## Stopping

```bash
docker compose down -v
```

The `-v` flag removes the named volumes (TimescaleDB data + log file). Omit it
to keep data across restarts.
