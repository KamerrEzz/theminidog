# miniobserv-express

A production-grade Express + TypeScript Tasks API that uses [@kamerrezz/miniobserv](https://github.com/KamerrEzz/theminidog) as its observability layer. Intended as a real-world integration example — not a tutorial.

## What this demonstrates

Two complementary integration patterns:

**Agent — system metrics (automatic)**
The `agent` container runs alongside your app and pushes CPU, memory, and disk metrics to MiniObserv on a fixed interval. Zero code required in the application itself.

**SDK `pushLog()` — application-level logs (intentional)**
The app calls `client.pushLog()` directly to record structured events: every HTTP request (method, path, status, latency), task creation, completion, and deletion. These are the logs that matter for understanding application behavior — not just infrastructure health.

## Quick start

```bash
docker compose up --build
```

- API: http://localhost:3000
- MiniObserv dashboard: http://localhost:8080

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Liveness check |
| `GET` | `/tasks` | List all tasks |
| `POST` | `/tasks` | Create a task `{ "title": "..." }` |
| `PATCH` | `/tasks/:id/done` | Mark a task as done |
| `DELETE` | `/tasks/:id` | Delete a task |

## What you see in MiniObserv

Every action produces a structured log entry visible in the dashboard:

- `GET /tasks 200 4ms` — HTTP traffic, sourced as `http`
- `Task created: "Write docs"` — business event, sourced as `tasks`
- `Task completed: "Write docs"` — info level
- `Task deleted: "Write docs"` — warn level (intentional signal for deletions)

Log levels map to HTTP status automatically: `info` for 2xx, `warn` for 4xx, `error` for 5xx.

## Local development (without Docker)

```bash
cp .env.example .env
npm install
npm run dev
```

Requires a running MiniObserv server. See the [MiniObserv docs](https://kamerrezz.github.io/theminidog/) for how to start one.

## Links

- Docs: https://kamerrezz.github.io/theminidog/
- Repo: https://github.com/KamerrEzz/theminidog
