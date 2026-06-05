# 01 — curl Quickstart

Demonstrates the complete MiniObserv API lifecycle in a single shell script using only `curl` and Python's standard library. No external packages needed.

## What it does

| Step | Action |
|------|--------|
| 1 | `GET /healthz` — verifies the server is alive |
| 2 | Mints a HS256 JWT signed with `AGENT_TOKEN` using `hmac` + `hashlib` from Python stdlib |
| 3 | `POST /api/v1/metrics` — pushes 5 different metric types for host `demo-host` |
| 4 | `GET /api/v1/metrics/query` — queries `cpu.usage_pct` for the last 5 minutes |
| 5 | Pretty-prints the result as a formatted table |

## Prerequisites

- `bash`
- `curl`
- `python3` (stdlib only, no pip packages needed)
- A running MiniObserv server

## How to run

```sh
export MINIOBSERV_URL=http://localhost:8080
export AGENT_TOKEN=your-secret-here-min-16-chars
bash run.sh
```
