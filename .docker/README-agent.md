# MiniObserv — Agent

The lightweight collector component of [MiniObserv](https://github.com/KamerrEzz/theminidog) — a self-hosted observability platform built in Go 1.23+.

Runs on each server you want to monitor. Collects CPU, memory, disk, and network metrics every 10 seconds. Tails log files. Ships everything via authenticated HTTP to the MiniObserv server.

**No database. No open ports. No dependencies.** Just two environment variables and it works.

## Quick start

```bash
docker run -d \
  -e SERVER_URL="http://your-miniobserv-server:8080" \
  -e AGENT_TOKEN="your-32-char-secret" \
  kamerrezz/miniobserv-agent:latest
```

## With log tailing

```bash
docker run -d \
  -e SERVER_URL="http://your-miniobserv-server:8080" \
  -e AGENT_TOKEN="your-32-char-secret" \
  -e LOG_PATHS="/var/log/app/server.log" \
  -v /var/log/app:/var/log/app:ro \
  kamerrezz/miniobserv-agent:latest
```

## Docker Compose (alongside your app)

```yaml
services:
  your-app:
    image: your-app:latest
    volumes:
      - app_logs:/var/log/app

  agent:
    image: kamerrezz/miniobserv-agent:latest
    environment:
      SERVER_URL: "http://miniobserv:8080"
      AGENT_TOKEN: "your-secret"
      LOG_PATHS: "/var/log/app/server.log"
      AGENT_HOST: "web-01"   # optional: override hostname
    volumes:
      - app_logs:/var/log/app:ro

volumes:
  app_logs:
```

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `SERVER_URL` | ✅ | — | MiniObserv server base URL |
| `AGENT_TOKEN` | ✅ | — | HS256 signing secret (same as server) |
| `COLLECT_INTERVAL` | | `10s` | Metric collection frequency |
| `AGENT_HOST` | | OS hostname | Host label for all metrics and logs |
| `LOG_PATHS` | | — | Comma-separated log file paths to tail |
| `LOG_LEVEL` | | `info` | `debug` / `info` / `warn` / `error` |

## Collected metrics

| Name | Description |
|---|---|
| `cpu.usage_pct` | CPU usage % (all cores) |
| `mem.used_pct` | Memory used % |
| `mem.used_bytes` | Memory used in bytes |
| `mem.total_bytes` | Total memory in bytes |
| `disk.used_pct` | Disk used % (root mount) |
| `disk.used_bytes` | Disk used in bytes |
| `disk.total_bytes` | Total disk in bytes |
| `net.bytes_in` | Network bytes received (delta) |
| `net.bytes_out` | Network bytes sent (delta) |

## How it works

```
Every 10 seconds:
  1. Read system metrics via gopsutil
  2. Tail any new lines from configured log files
  3. Mint HS256 JWT from AGENT_TOKEN (cached 24h)
  4. POST /api/v1/metrics to server (batch)
  5. POST /api/v1/logs to server (batch)
```

The agent is **stateless** — it holds no data locally. If it restarts, it begins collecting fresh. The server marks it as `stale` after 20s of silence and `down` after 50s, firing a webhook notification if configured.

## Links

- **Source**: [github.com/KamerrEzz/theminidog](https://github.com/KamerrEzz/theminidog)
- **Docs**: [kamerrezz.github.io/theminidog](https://kamerrezz.github.io/theminidog/)
- **Server image**: [kamerrezz/miniobserv-server](https://hub.docker.com/r/kamerrezz/miniobserv-server)
- **TypeScript SDK**: [@kamerrezz/miniobserv](https://www.npmjs.com/package/@kamerrezz/miniobserv)
