# Agent vs Server — Why Two Binaries?

This is one of the first questions you'll have when you look at this project. There are two separate Go binaries: `agent` and `server`. Why not just one program that does everything?

The short answer: **the agent runs on every machine you want to monitor. The server runs once, in the center.**

---

## The mental model

Imagine you have 5 servers running your application:

```
  web-01          web-02          worker-01
┌──────────┐    ┌──────────┐    ┌──────────┐
│  [agent] │    │  [agent] │    │  [agent] │
│          │    │          │    │          │
│ cpu: 42% │    │ cpu: 71% │    │ cpu: 18% │
│ mem: 6GB │    │ mem: 7GB │    │ mem: 2GB │
└────┬─────┘    └────┬─────┘    └────┬─────┘
     │               │               │
     └───────────────┴───────────────┘
                     │  HTTP POST /api/v1/metrics
                     ▼
              ┌─────────────┐
              │   [server]  │──── TimescaleDB
              │             │
              │  dashboard  │◄─── your browser
              │  alerting   │──── Slack webhook
              │  query API  │
              └─────────────┘
```

**One server. Many agents.** This is the same model Datadog uses — they call it the Datadog Agent and the Datadog backend. The difference is that with MiniObserv, you own and run the server yourself.

---

## What the Agent does

The agent is a **collector and shipper**. It has no database, no web server, no dashboard. Its only job is:

1. **Collect** — every 10 seconds, reads CPU usage, memory, disk, and network from the OS
2. **Tail** — watches log files for new lines (using `fsnotify`)
3. **Ship** — batches everything and POSTs to the server with a JWT

```
Agent loop (every 10s):
  ┌─────────────────────────────────────────┐
  │  read CPU from /proc/stat               │
  │  read mem from /proc/meminfo            │
  │  read disk from syscall                 │
  │  read network deltas from /proc/net/dev │
  │  read new log lines from tailed files   │
  │                                         │
  │  → batch everything                     │
  │  → POST /api/v1/metrics  (JWT)          │
  │  → POST /api/v1/logs     (JWT)          │
  └─────────────────────────────────────────┘
```

The agent is **stateless between collections**. If it crashes and restarts, it just starts collecting again. No data is stored locally.

The agent binary is small (~10 MB). You can drop it on any Linux server, set two environment variables, and it starts working.

```bash
SERVER_URL=http://your-server:8080 \
AGENT_TOKEN=your-secret \
./agent
```

---

## What the Server does

The server is the **brain**. It receives data from all agents, stores it, and serves it back. It has five responsibilities:

| Responsibility | How |
|---|---|
| **Receive metrics and logs** | POST endpoints, validates JWT, bulk-inserts via `pgx.Batch` |
| **Store** | TimescaleDB hypertable (metrics), plain table (logs) |
| **Query** | Time-bucket API, keyset-paginated log search |
| **Alert** | 30s ticker evaluates threshold rules against latest data |
| **Serve dashboard** | Embeds HTML/JS with `//go:embed`, no build step |

The server needs a database (TimescaleDB). The agent does not. This is why they are separate: you don't want to install a database on every machine you're monitoring.

---

## Why not one binary?

You might think: "why not combine them?" Here's what would break:

**1. You'd have to install a database on every server**

Every machine running the combined binary would need TimescaleDB. A monitoring agent should be lightweight and have no external dependencies. The agent today has zero: no database, no disk writes, no ports open.

**2. You'd lose the N-to-1 aggregation**

With separate binaries, 50 agents send to 1 server. With a combined binary, each machine would only see its own data. The whole point of a monitoring system is seeing all your machines in one place.

**3. Security would be weaker**

The agent only sends data — it never reads it back. If an agent is compromised, the attacker can inject fake metrics but can't read your monitoring history. If you combined them, every agent would have read access to everything.

**4. Network topology would break**

Agents push data outbound (port 8080 on the server). They don't need any inbound ports open. This works even when agents are behind firewalls or NAT. A combined binary would need every machine to accept inbound connections.

---

## The JWT handshake

Agents don't have usernames or passwords. They use a **shared HS256 signing secret** (`AGENT_TOKEN`). When the agent starts, it uses this secret to mint a signed JWT valid for 24 hours. The server verifies the signature on every request.

```
Agent                           Server
  │                               │
  │  AGENT_TOKEN = "my-secret"    │  AGENT_TOKEN = "my-secret"
  │                               │
  │  mint JWT (HS256, 24h TTL)    │
  │                               │
  │── POST /api/v1/metrics ──────►│
  │   Authorization: Bearer <JWT> │  verify signature
  │                               │  ✓ same secret → accept
  │◄─ 202 Accepted ───────────────│
```

Both the agent and server are configured with the same `AGENT_TOKEN`. The agent uses it to sign tokens; the server uses it to verify them. Neither stores passwords or certificates.

---

## Practical: running both

**Development (same machine):**
```bash
# Terminal 1 — server
DATABASE_URL=postgres://... AGENT_TOKEN=dev-secret ./server

# Terminal 2 — agent (pointing to local server)
SERVER_URL=http://localhost:8080 AGENT_TOKEN=dev-secret ./agent
```

**Production (separate machines):**
```bash
# On your monitoring server
DATABASE_URL=postgres://... AGENT_TOKEN=prod-secret ./server

# On each server you want to monitor
SERVER_URL=http://monitoring-server:8080 AGENT_TOKEN=prod-secret ./agent
```

**Docker Compose (both in one stack, same network):**
```yaml
services:
  server:
    image: kamerrezz/miniobserv-server:latest
    environment:
      DATABASE_URL: postgres://minidog:minidog@db:5432/miniobserv
      AGENT_TOKEN: your-secret
    ports: ["8080:8080"]

  agent:
    image: kamerrezz/miniobserv-agent:latest
    environment:
      SERVER_URL: http://server:8080   # ← service name in Docker network
      AGENT_TOKEN: your-secret
```

---

## Summary

| | Agent | Server |
|---|---|---|
| **Runs on** | Every machine you monitor | One central machine |
| **Count** | N (one per host) | 1 |
| **Has database** | No | Yes (TimescaleDB) |
| **Opens ports** | No | Yes (8080) |
| **Binary size** | ~10 MB | ~15 MB |
| **State** | Stateless | Stateful |
| **Restartable** | Instantly | Needs DB connection |
| **Docker image** | `kamerrezz/miniobserv-agent` | `kamerrezz/miniobserv-server` |

The split is not a design quirk — it is the only design that scales to monitoring multiple machines. Every real observability platform (Datadog, Prometheus, Grafana Cloud, Elastic) uses the same pattern: a lightweight collector per host and a centralized backend that aggregates everything.
