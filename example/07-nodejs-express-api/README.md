# miniobserv-express

A production-grade **Express + TypeScript + Prisma + PostgreSQL** Tasks API with JWT authentication and [@kamerrezz/miniobserv](https://github.com/KamerrEzz/theminidog) observability.

## Stack

- **Express.js + TypeScript** — HTTP layer
- **Prisma ORM + PostgreSQL** — persistent storage, schema migrations
- **jsonwebtoken + bcrypt** — JWT authentication, password hashing
- **@kamerrezz/miniobserv** — structured log shipping + system metrics

---

## Quick start (Docker)

```bash
docker compose up --build
```

- API: http://localhost:3000
- MiniObserv dashboard: http://localhost:8080

The container automatically runs `prisma migrate deploy` before starting the server.

---

## API reference

All protected endpoints require `Authorization: Bearer <token>`.

### Auth

**Register**
```bash
curl -X POST http://localhost:3000/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"name":"Alice","email":"alice@example.com","password":"secret123"}'
```

**Login — get your token**
```bash
curl -X POST http://localhost:3000/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"alice@example.com","password":"secret123"}'
# → { "token": "<jwt>", "user": { ... } }
```

### Tasks (JWT required)

**List tasks**
```bash
curl http://localhost:3000/tasks \
  -H 'Authorization: Bearer <token>'
```

**Create a task**
```bash
curl -X POST http://localhost:3000/tasks \
  -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' \
  -d '{"title":"Ship the feature"}'
```

**Mark done**
```bash
curl -X PATCH http://localhost:3000/tasks/<id>/done \
  -H 'Authorization: Bearer <token>'
```

**Delete**
```bash
curl -X DELETE http://localhost:3000/tasks/<id> \
  -H 'Authorization: Bearer <token>'
```

### Health (public)

```bash
curl http://localhost:3000/health
# → { "status": "ok", "ts": "..." }
```

---

## The three layers of observability

Observability in a real app has three distinct layers. It's important to know which layer each tool covers:

```
Layer 1 — Infrastructure        ← MiniObserv lives here
  Is the machine healthy?
  CPU, memory, disk, network, system logs
  Tools: MiniObserv, Datadog, Prometheus + Grafana

Layer 2 — Application (APM)
  Is my code healthy?
  Slow queries, errors, stack traces, N+1, latency per endpoint
  Tools: Sentry, Datadog APM, OpenTelemetry

Layer 3 — Business
  Is my product healthy?
  Active users, conversions, feature usage, revenue
  Tools: PostHog, Mixpanel, Amplitude
```

**MiniObserv covers Layer 1.** It monitors the server running your app — not your Prisma schema, not your user table, not your business logic. When you ship logs with `obs.request()` or `obs.info()`, you're feeding Layer 1: infrastructure signals (latency, error rate, what happened when). That's different from knowing *why* a user churned (Layer 3) or *which* Prisma query caused a spike (Layer 2).

In production you typically run all three. Start with Layer 1 — if the server runs out of memory, Layers 2 and 3 don't matter.

---

## How observability works

Two mechanisms run side by side:

**1. `httpLogger` middleware** (`src/middleware/httpLogger.ts`)
Hooks `res.on('finish')` on every request and calls `obs.request(method, path, status, ms)`. Every HTTP hit is logged to MiniObserv as a structured entry — method, path, status code, latency. No route needs to care about this.

**2. `obs` helpers in routes** (`src/observability.ts`)
Routes call `obs.info`, `obs.warn`, `obs.error` directly to record business events:
- `User registered: alice@example.com` (source: `auth`)
- `User logged in: alice@example.com` (source: `auth`)
- `Task created: "Ship the feature"` (source: `tasks`)
- `Task completed: "Ship the feature"` (source: `tasks`)
- `Task deleted: "..."` (source: `tasks`, level `warn` — intentional signal)

Log levels on HTTP events are automatic: `info` for 2xx, `warn` for 4xx, `error` for 5xx.

---

## Two databases — why

```
app DB (postgres:16)           miniobserv DB (timescaledb)
tasks_db                       miniobserv
  └── users                      └── logs, metrics, alerts
  └── tasks
```

These are completely isolated. The app database holds your application data (users, tasks) and is managed by Prisma migrations. The MiniObserv database is owned by the MiniObserv server and holds structured logs and time-series system metrics. They share no schema, no connection, and no data. Keeping them separate means a MiniObserv outage never affects your app, and your app's database load never competes with observability writes.

---

## Development without Docker

**Prerequisites:** Node 20+, a running PostgreSQL instance.

```bash
# 1. Copy env and fill in your local values
cp .env.example .env

# 2. Install dependencies and generate Prisma client
npm install

# 3. Create the database and run migrations
npx prisma migrate dev --name init

# 4. Start the dev server with hot reload
npm run dev
```

For the MiniObserv sidecar locally, run only those services via Docker:

```bash
docker compose up miniobserv-db miniobserv miniobserv-agent
```

Then the app will connect to `http://localhost:8080` as configured in `.env`.

---

## Environment variables

| Variable | Description | Default |
|---|---|---|
| `PORT` | HTTP port | `3000` |
| `DATABASE_URL` | PostgreSQL connection string | — |
| `JWT_SECRET` | Token signing key | `dev-secret` |
| `APP_HOST` | Host label in MiniObserv logs | `tasks-api` |
| `MINIOBSERV_URL` | MiniObserv server base URL | `http://localhost:8080` |
| `MINIOBSERV_TOKEN` | Agent token for MiniObserv | `dev-secret` |

Generate a strong `JWT_SECRET` for production:
```bash
openssl rand -hex 32
```

---

## Links

- Docs: https://kamerrezz.github.io/theminidog/
- Repo: https://github.com/KamerrEzz/theminidog
