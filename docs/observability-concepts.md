# Observability Concepts — A Beginner's Guide

> This guide is for people who have heard words like "metrics", "logs", "Grafana", or "Datadog" and want to actually understand what they mean — and why they matter. We'll use MiniObserv as our concrete example throughout.

---

## 🚗 What is Observability?

You write software. You deploy it. Then what?

How do you know it's actually working? How do you know it slowed down at 2am last Tuesday? How do you know a crash happened while you were asleep?

Without observability, you're **flying blind**. The application is running somewhere on a server, and your only option is to wait for a user to complain.

There are different levels of visibility you can have into a running system:

**Flying blind** — no monitoring at all. You find out something is broken when users email you.

**Looking in the rearview mirror** — you have logs. You can investigate *after* something goes wrong, but you can't see a problem developing in real time.

**Full dashboard** — you have metrics, logs, and ideally traces. You can see problems as they happen, understand trends, and get alerted automatically.

Think of it like driving a car. A **speedometer** tells you how fast you're going right now — that's a metric. An **engine warning light** means something crossed a threshold you care about — that's an alert. A **mechanic's service log** records what happened and when — that's a log.

Observability is the combination of all three. It's the practice of making your software tell you what it's doing.

---

## 📐 The Three Pillars

The industry has settled on three fundamental building blocks:

**Metrics** are numbers measured over time. "CPU usage was 87% at 14:00." They're compact, fast to store, and great for spotting trends or setting alerts. They answer: *how much, how often, how long?*

**Logs** are text records of events. "User 123 logged in at 14:32:01." They're verbose, but they capture *why* something happened. They answer: *what exactly occurred, and in what order?*

**Traces** are the path a single request takes as it moves through multiple services. "This HTTP request went: load balancer → API server → database → cache." Traces answer: *where did this request spend its time?*

MiniObserv covers **metrics and logs**. Traces are a more advanced topic — they require every service in your system to participate and pass around a shared "trace ID". They're essential for large distributed systems, but overkill for a single-server setup like ours.

---

## 📊 What is a Metric?

A metric is a single measured number, with four pieces of information attached:

- **Name** — what you're measuring (`cpu.usage_pct`)
- **Value** — the actual number (`42.5`)
- **Timestamp** — when you measured it (`2026-06-05T10:00:00Z`)
- **Labels** — extra context to distinguish *which* of something (`core=total`, or `mount=/`, or `host=web-01`)

Here's a real metric that MiniObserv collects and stores:

```
cpu.usage_pct{host="web-01", core="total"} = 42.5
  at 2026-06-05T10:00:00Z
```

That tells you: on the host named `web-01`, total CPU usage across all cores was 42.5% at exactly 10am.

### Gauge vs. Counter vs. Histogram

You'll encounter three common metric types:

A **gauge** is a snapshot of a current value that can go up or down. CPU usage is a gauge. Memory usage is a gauge. The number changes in either direction from moment to moment.

A **counter** only goes up. "Number of HTTP requests served" is a counter — it starts at zero and grows forever. Counters are useful for measuring rates: "how many requests per second in the last minute?"

A **histogram** records a distribution of values — how many requests took 0–10ms, how many took 10–50ms, how many took 50–200ms, and so on. Useful for understanding latency.

MiniObserv collects gauges: CPU percentage, memory used, disk used, network bytes.

### Why Not Just Store Everything in a Regular Database?

This is where it gets interesting. The agent collects a measurement every 10 seconds. On a machine with a CPU and 4 cores, that's 5 data points per collection cycle. Over 24 hours, that's 43,200 rows. Over 30 days for 10 hosts, you're in the millions.

When you query that data, you almost never want every raw row. You want answers like: "What was the average CPU every 5 minutes over the last hour?" Instead of 360 raw rows, that query should return 12 data points.

This is what `time_bucket` does — a function from **TimescaleDB** that groups rows into time windows and aggregates them:

```
Every 10 seconds, the agent stores:
  10:00:00 → 42.5%
  10:00:10 → 43.1%
  10:00:20 → 41.8%
  ...  (30 rows per 5-minute window)

Query: avg CPU per 5-min window, last hour
  → 12 points, clean and readable
```

TimescaleDB is an extension for PostgreSQL that understands time-series data natively. It automatically cuts the table into **chunks** by time period — so when you query "last hour", it only reads the chunk that covers that hour instead of scanning millions of rows. More on this below.

---

## 📋 What is a Log?

A log is a text message your application writes when something notable happens. Unlike a metric (a number), a log is a description.

Every log line has:

- **Time** — when it happened
- **Level** — how severe it is
- **Message** — what happened
- **Context** — extra fields that help you understand it

Log levels form a hierarchy from noisy to critical:

- `DEBUG` — extremely detailed, usually only turned on during development
- `INFO` — normal operations ("server started", "user logged in")
- `WARN` — something unusual happened but the system is still working
- `ERROR` — something broke

```
2026-06-05T14:32:01Z INFO  user login successful user_id=123
2026-06-05T14:32:15Z ERROR database query failed err="connection timeout"
```

Here's the key insight about why you need both metrics and logs: **metrics tell you that something is wrong, logs tell you why**.

If your `cpu.usage_pct` metric spikes to 98%, that's your warning light. But the metric alone doesn't explain what caused it. The logs from that same window might show "database query took 4200ms" and "retrying connection" — now you have a lead.

---

## 🗺️ How MiniObserv Works — The Full Picture

Here's the complete flow, from your machine to your query:

```
Your server / computer
       │
   [AGENT]
   Every 10 seconds:
     - Reads CPU, RAM, disk, network from the OS
     - Packages them into a batch
       │
       │  HTTP POST (over the network)
       │  Authorization: Bearer <JWT token>
       │
       ▼
   [SERVER]
   Receives the batch
   Validates the JWT token
   Validates the metric data
   Stores metrics → TimescaleDB hypertable
       │
       ▼
   [YOU, querying later]
   GET /api/v1/metrics/query?host=web-01&name=cpu.usage_pct
     &from=...&to=...&bucket=5m&agg=avg
   ← Receives time-bucketed averages
```

**The Agent** is a lightweight program that runs on your machine. It wakes up on a timer (every 10 seconds by default), asks the operating system for current resource stats, and ships them to the server as a batch. It has no UI and no database — it just collects and sends.

**The Server** receives those batches, checks that the sender is authorized, validates the data, and writes it to the database. It also exposes the query API so you can retrieve data later.

**TimescaleDB** is a PostgreSQL extension that handles the storage. It knows how to store millions of time-series rows efficiently and query them quickly using `time_bucket`.

---

## 🗄️ Why TimescaleDB for Metrics?

Imagine storing a year of CPU readings for 20 servers, collected every 10 seconds. That's over 60 million rows. If you used a plain PostgreSQL table, every query like "show me the last hour" would potentially scan through millions of irrelevant rows from months ago.

TimescaleDB solves this with **hypertables**. A hypertable looks exactly like a regular PostgreSQL table — you `INSERT` and `SELECT` from it the same way — but TimescaleDB quietly splits it into **chunks** behind the scenes, each covering a fixed time window.

Think of a filing cabinet. A regular table is one giant drawer where you throw every document in chronological order. A hypertable is a cabinet where each drawer is labeled by week — "January W1", "January W2", etc. When you ask "give me everything from last Tuesday", the system goes straight to the right drawer.

In MiniObserv, the `metrics` table is a hypertable. The `time_bucket` function in the query layer then does the aggregation:

```sql
SELECT time_bucket('5 minutes', time) AS bucket,
       avg(value) AS value
FROM metrics
WHERE host = 'web-01'
  AND name = 'cpu.usage_pct'
  AND time >= '...' AND time <= '...'
GROUP BY bucket
ORDER BY bucket DESC
```

TimescaleDB only reads the chunks that overlap your time range, aggregates the data, and hands back a small, clean result.

---

## 🔐 Why JWT for Authentication?

The agent runs on a separate machine and needs to prove to the server that it's allowed to send data. Without this, anyone who discovered your server's address could flood it with fake metrics.

A **JWT** (JSON Web Token) solves this without a traditional login. It's a signed message the agent attaches to every HTTP request.

Think of it like a wax seal on a letter. Anyone can read the letter, but only someone with the original signet ring can create a new seal that looks authentic. If the wax seal is cracked or missing, you throw the letter out.

In MiniObserv, the agent and the server share a **secret key**. The agent signs its JWT with that key using **HMAC-SHA256** — a mathematical function that's trivial to verify (if you know the key) but practically impossible to fake without it. The server checks every incoming request for a valid Bearer token before touching the data.

The server enforces `HS256` specifically — this blocks a known attack called **alg=none**, where a malicious client claims the token needs no signature at all.

No username. No password. Just a shared secret and a math function.

---

## 🤖 What is an Agent?

In monitoring, an **agent** is a program that runs on a machine and reports data to a central server. It's deliberately lightweight — it should use as little CPU and memory as possible so it doesn't interfere with the actual application you're monitoring.

There are two common patterns:

**Push model** — the agent wakes up on a schedule and sends data to the server. MiniObserv uses this. The agent drives the conversation.

**Pull model** — the server periodically reaches out and asks each agent for its data. Prometheus (a popular open-source monitoring tool) uses this model.

The push model suits MiniObserv well: the agent knows when it collected data, and it handles retries with exponential backoff if the server is temporarily unavailable.

The MiniObserv agent uses **gopsutil**, a Go library that provides a cross-platform interface to OS statistics. It calls the same data that `htop` or Task Manager would show you — CPU, memory, disk, and network — and packages it into the `model.Metric` format.

---

## 🔌 Understanding the API

Two HTTP endpoints do all the work.

**Ingestion** — the agent calls this every collection cycle:

```
POST /api/v1/metrics
Authorization: Bearer <JWT>
Content-Type: application/json

{
  "host": "web-01",
  "metrics": [
    { "time": "2026-06-05T10:00:00Z", "name": "cpu.usage_pct",
      "value": 42.5, "labels": {"core": "total"} },
    { "time": "2026-06-05T10:00:00Z", "name": "mem.used_pct",
      "value": 61.2, "labels": {} }
  ]
}

← 202 Accepted
   {"ingested": 2}
```

The server validates the JWT, validates every metric in the batch, and writes them to TimescaleDB. It responds `202 Accepted` — meaning "I got it and I'll process it", not "it's already done."

**Querying** — you call this to retrieve data:

```
GET /api/v1/metrics/query
  ?host=web-01
  &name=cpu.usage_pct
  &from=2026-06-05T09:00:00Z
  &to=2026-06-05T10:00:00Z
  &bucket=5m
  &agg=avg
Authorization: Bearer <JWT>

← 200 OK
{
  "host": "web-01",
  "name": "cpu.usage_pct",
  "bucket": "5m",
  "agg": "avg",
  "points": [
    { "time": "2026-06-05T09:55:00Z", "value": 44.1 },
    { "time": "2026-06-05T09:50:00Z", "value": 42.8 },
    ...
  ]
}
```

The server reads one hour of raw data, groups it into 5-minute buckets, averages the values in each bucket, and returns 12 clean data points.

Supported bucket sizes: `1m`, `5m`, `15m`, `1h`, `1d`.
Supported aggregations: `avg`, `max`, `min`.

---

## 📖 Glossary

**Observability** — the ability to understand the internal state of a system by looking at its outputs (metrics, logs, traces). If you can ask any question about your system without deploying new code, it's observable.

**Metric / Time-series** — a numerical measurement captured at a specific point in time. A time-series is a sequence of such measurements for the same thing over time.

**Gauge** — a metric type that records a current value which can go up or down (CPU %, memory used, active connections).

**Counter** — a metric type that only increases (total HTTP requests, total bytes sent). Useful for computing rates.

**Log / Log level** — a text record of an event. Log levels (`DEBUG`, `INFO`, `WARN`, `ERROR`) indicate severity so you can filter noise from signal.

**Agent** — a program that runs on a machine and reports data to a central server on a schedule.

**Hypertable** — a TimescaleDB-managed table that is automatically partitioned by time into chunks. Queries over recent data only touch the relevant chunks instead of the entire table.

**Chunk** — one time-bounded partition of a hypertable. TimescaleDB creates and manages these automatically.

**JWT (JSON Web Token)** — a signed, self-contained token used for authentication. The signature is produced using a shared secret, so the server can verify authenticity without storing session state.

**HMAC-SHA256** — the cryptographic function used to sign JWTs in MiniObserv. It produces a signature that is easy to verify with the key but practically impossible to forge without it.

**Ingestion** — the act of receiving and storing incoming data. In MiniObserv, ingestion happens when the server accepts a `POST /api/v1/metrics` request from an agent.

**Cardinality** — the number of unique label combinations for a metric. `cpu.usage_pct{host="web-01", core="total"}` and `cpu.usage_pct{host="web-01", core="0"}` are two distinct series — two units of cardinality. High cardinality (millions of unique label combinations) is a common performance problem in monitoring systems because each unique combination requires its own storage and query path. In MiniObserv, labels are kept intentionally small: `core` for CPU, `mount` for disk.

**Pull vs. Push** — two models for collecting data. In the push model, the agent sends data to the server on a schedule (MiniObserv). In the pull model, the server queries each agent for data on its own schedule (Prometheus). Each has tradeoffs around firewall traversal, discovery, and backpressure.

**time_bucket** — a TimescaleDB SQL function that rounds a timestamp down to the start of its containing window (e.g., `time_bucket('5 minutes', '10:07:32')` returns `10:05:00`). Used in queries to group raw samples into readable summaries.
