# How We Built This — A 5-Week Journey

## The question that started it all

"What if I built a mini Datadog?"

Not to replace it. Not to compete with it. But to understand it: how does a metrics pipeline actually work end to end? What does it take to collect CPU and memory from a machine, ship it over a network, persist it in a time-series database, and show it on a live dashboard?

The answer was to build it from scratch. No existing code to copy from, no shortcuts, no scaffold generators. The goal was learning, not shipping fast. Every week, one layer of the system went from "I don't know how this works" to "I built it, I know exactly how it works."

That project is MiniObserv.

---

## Spec-Driven Development (SDD)

Before a single line of code was written, every week started with a planning phase. The sequence was always the same:

```
explore → propose → spec ──┐
                            ├──► tasks → apply → verify → archive
                       design ──┘
```

**Explore** means reading the problem space without commitment: what are the constraints? what have others done? what are the tradeoffs?

**Propose** means picking a direction and writing it down as a short proposal: here is what we will build, here is why, here are the alternatives we rejected.

**Spec** turns the proposal into a contract: what inputs does the system accept, what outputs does it produce, what are the invariants?

**Design** maps the spec onto concrete structures: which packages, which interfaces, which types, which SQL schema?

**Tasks** is the breakdown: a numbered list of small, testable units of work in dependency order.

**Apply** is implementation: one task at a time, test first.

**Verify** checks the implementation against the spec: does everything that was promised actually exist and behave correctly?

**Archive** closes the change: the spec, the design, the apply log, and the verify report all go into the record.

Why does this matter? When you write the spec first, you catch design mistakes before they become code mistakes. Specs are cheap to change. A sentence in a spec costs nothing to delete. A function signature that has propagated across ten files costs hours to rename. The discipline of specifying first forces you to think through the system before you commit to a shape.

---

## Strict TDD — Red, Green, Refactor

Every task that touched Go code followed the same rhythm:

1. **RED** — Write a failing test that describes exactly what the code should do. Run it. Watch it fail. The failure message should be specific.
2. **GREEN** — Write the minimum code that makes the test pass. Not the cleanest code. Not the most general code. Just enough to make the test green.
3. **Refactor** — Clean up. Remove duplication. Improve names. The test suite keeps you safe: if you break something during cleanup, a test fails immediately.

The rule was strict: never write implementation code without a failing test first. If you find yourself writing a function and then writing tests for it afterward, you have already lost the benefit. The test is not a formality — it is the specification in executable form.

The result: 213 tests across the codebase, zero flaky tests, and the ability to refactor the storage layer, the evaluator, or the collector pipeline with confidence that the test suite will catch any regression.

---

## Week by week

### Week 1 — Agent

The agent collects system metrics (CPU, memory, disk, network) using gopsutil, accumulates them into batches, and ships them to the server via HTTP/JSON every 10 seconds. The key decisions here were delta semantics for network counters, function injection for testability without OS syscalls, and a buffered channel to decouple collection from sending.

### Week 2 — Server

The server receives metrics over HTTP, authenticates agents with JWT HS256, and persists data to TimescaleDB. The query API lets clients ask for time-bucketed aggregations over any time range. The critical decisions here were the narrow table model, `pgx.Batch` for bulk inserts, and allowlist interpolation for `time_bucket` parameters to avoid pgx plan-cache collisions.

### Week 3 — Logs

The agent gained a log tail capability: it reads application log files line by line using `bufio.Scanner`, ships entries to the server via the ingest API, and the server stores them in a TimescaleDB hypertable. The search API supports full-text filtering with pagination. Migration 003 introduced the composite primary key requirement that TimescaleDB imposes on hypertables.

### Week 4 — Dashboard and Alerting

A live web dashboard was added using `html/template` and `embed` — no JavaScript framework, no frontend build step. Threshold alert rules let operators define conditions like "cpu.percent > 90 for 5 consecutive evaluations". The evaluator runs on a ticker, queries recent metrics, and fires alerts when thresholds are crossed. Zero new external dependencies.

### Week 5 — Notifications, Host Health, and Retention

The final week added webhook notifications for threshold alerts and host-down events, a `HostTracker` for real-time liveness monitoring, and TimescaleDB retention policies to cap storage growth. `context.WithoutCancel` ensured that notification goroutines survive the ingest handler's lifecycle. The functional option `WithNotifiers` kept all 213 existing tests intact.

---

## Architecture Decision Records

Every non-obvious decision has an ADR. Not "what we did" — the code already shows that. The ADRs document *why*, and specifically what the alternatives were and why they were rejected.

23 ADRs for 5 weeks.

Some examples of why this matters:

- **ADR-6** explains why `time_bucket` parameters are string-interpolated from an allowlist instead of parameterized with `$1::interval`. Without the ADR, a future developer would see the `fmt.Sprintf` and reasonably conclude it was a security oversight.
- **ADR-22** explains why the `logs` table has a composite primary key `(id, time)` instead of just `id`. Without the ADR, it looks like an unnecessary complication.
- **ADR-5** documents the `defer br.Close()` requirement for `pgx.Batch`. Without it, a refactor that removes the defer would silently exhaust the connection pool under load.

When you come back to this code in six months, or when someone new joins the project, the ADRs answer the "why" questions that the code cannot.

---

## Zero external dependencies for new features

Weeks 4 and 5 added alerting, a live dashboard, webhook notifications, and host health tracking using nothing but Go's standard library:

- `html/template` for server-side rendering
- `embed` for bundling templates and static assets into the binary
- `sync` for the mutex in `HostTracker`
- `context` for cancellation and timeout management
- `time` for tickers, timestamps, and liveness thresholds
- `encoding/json` for webhook payloads

No new imports. This is a deliberate choice.

When you reach for a library, you stop learning how the problem is actually solved. A library is a correct answer — but it hides the question. If you want to understand how a template engine works, build one. If you want to understand how a concurrency primitive works, implement it yourself first. Then use the library in production, with full awareness of what it is doing.

---

## If you want to build something like this

Building a system from scratch to understand it is one of the most effective learning strategies available. Some practical advice:

1. **Pick one system you use but do not understand.** Not the whole stack — one component. A metrics pipeline. An auth layer. A queue. A cache. A rate limiter.

2. **Ask: what would the simplest version look like?** Not the most scalable. Not the most production-ready. The simplest version that demonstrates the core idea. MiniObserv is not Datadog. It does not need to be.

3. **Write the spec before the code.** What data flows in? What data flows out? What are the invariants? Write it down. If you cannot explain it in plain sentences, you do not understand it well enough to implement it.

4. **Add tests before adding features.** Every new behavior has a test. The test suite is the executable version of the spec.

5. **Ship something, even if it is incomplete.** The act of deploying, watching it run, and seeing it fail in unexpected ways teaches you things the spec never will. Week 1's agent was incomplete. Week 2's server had no dashboard. Week 5 added things that week 1 did not anticipate. That is not a problem — it is how real systems are built.

---

This project is open source and was built to be read, not just used. If something is unclear, the ADRs explain the why. If you want to extend it, the internals guide shows you where.
