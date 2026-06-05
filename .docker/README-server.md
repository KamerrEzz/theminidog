# MiniObserv — Server

The central component of [MiniObserv](https://github.com/KamerrEzz/theminidog) — a self-hosted observability platform built in Go 1.23+.

Receives metrics and logs from agents, stores them in TimescaleDB, evaluates threshold alert rules, fires webhook notifications, and serves a live dark-theme dashboard.

## Quick start

```bash
docker run -d \
  -e DATABASE_URL="postgres://minidog:minidog@your-db:5432/miniobserv?sslmode=disable" \
  -e AGENT_TOKEN="your-32-char-secret" \
  -p 8080:8080 \
  kamerrezz/miniobserv-server:latest
```

Open **http://localhost:8080** for the live dashboard.

## Docker Compose (full stack)

```yaml
services:
  db:
    image: timescale/timescaledb:latest-pg16
    environment:
      POSTGRES_USER: minidog
      POSTGRES_PASSWORD: minidog
      POSTGRES_DB: miniobserv
    volumes:
      - miniobserv_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U minidog -d miniobserv"]
      interval: 5s
      retries: 10

  server:
    image: kamerrezz/miniobserv-server:latest
    environment:
      DATABASE_URL: "postgres://minidog:minidog@db:5432/miniobserv?sslmode=disable"
      AGENT_TOKEN: "your-secret"
      ALERT_RULES: '[{"host":"*","name":"cpu.usage_pct","op":">","threshold":80,"for":"5m"}]'
      ALERT_NOTIFICATIONS: '[{"type":"webhook","url":"https://hooks.slack.com/services/YOUR/WEBHOOK"}]'
    ports:
      - "8080:8080"
    depends_on:
      db:
        condition: service_healthy

  agent:
    image: kamerrezz/miniobserv-agent:latest
    environment:
      SERVER_URL: "http://server:8080"
      AGENT_TOKEN: "your-secret"
    depends_on:
      - server

volumes:
  miniobserv_data:
```

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | ✅ | — | PostgreSQL DSN (TimescaleDB) |
| `AGENT_TOKEN` | ✅ | — | HS256 signing secret shared with agents |
| `LISTEN_ADDR` | | `:8080` | Bind address |
| `ALERT_RULES` | | — | JSON array of threshold rules |
| `ALERT_NOTIFICATIONS` | | — | JSON array of webhook objects |
| `HOST_STALE_AFTER` | | `20s` | Duration before a silent host is marked stale |
| `HOST_DOWN_AFTER` | | `50s` | Duration before a silent host fires host.down |
| `LOG_LEVEL` | | `info` | `debug` / `info` / `warn` / `error` |

## Alert rules

```json
[
  {"host":"*","name":"cpu.usage_pct","op":">","threshold":80,"for":"5m"},
  {"host":"web-01","name":"mem.used_pct","op":">","threshold":85,"for":"10m"}
]
```

## Webhook notifications

```json
[
  {"type":"webhook","url":"https://hooks.slack.com/services/YOUR/WEBHOOK"},
  {"type":"webhook","url":"https://discord.com/api/webhooks/YOUR/WEBHOOK"}
]
```

Payload sent on FIRING and RESOLVED:
```json
{
  "event": "firing",
  "rule": {"Host":"*","Name":"cpu.usage_pct","Op":">","Threshold":80},
  "value": 83.4,
  "fired_at": "2026-06-05T16:42:23Z"
}
```

## Endpoints

| Endpoint | Auth | Description |
|---|---|---|
| `GET /` | — | Live dashboard |
| `GET /healthz` | — | Liveness probe |
| `GET /readyz` | — | Readiness + DB check |
| `GET /api/v1/alerts` | — | Active alert states |
| `GET /api/v1/hosts` | — | Host health status |
| `POST /api/v1/metrics` | JWT | Ingest metric batch |
| `POST /api/v1/logs` | JWT | Ingest log batch |
| `GET /api/v1/metrics/query` | JWT | Time-bucketed metric query |
| `GET /api/v1/logs/query` | JWT | Paginated log search |

## Links

- **Source**: [github.com/KamerrEzz/theminidog](https://github.com/KamerrEzz/theminidog)
- **Docs**: [kamerrezz.github.io/theminidog](https://kamerrezz.github.io/theminidog/)
- **Agent image**: [kamerrezz/miniobserv-agent](https://hub.docker.com/r/kamerrezz/miniobserv-agent)
- **TypeScript SDK**: [@kamerrezz/miniobserv](https://www.npmjs.com/package/@kamerrezz/miniobserv)
