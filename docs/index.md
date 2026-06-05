---
layout: home

hero:
  name: "MiniObserv"
  text: "Self-hosted observability"
  tagline: Collect metrics and logs from your servers, store them in TimescaleDB, fire alerts, and monitor everything from a live dashboard. Built in Go 1.23+.
  actions:
    - theme: brand
      text: Quick Start →
      link: /getting-started
    - theme: alt
      text: GitHub
      link: https://github.com/KamerrEzz/theminidog

features:
  - icon: 📊
    title: System Metrics
    details: CPU, memory, disk, and network collected every 10s by a lightweight Go agent using gopsutil. No dependencies on the monitored host.

  - icon: 📄
    title: Log Tailing
    details: Agent tails any log file with rotation detection. Log lines appear in the dashboard in real time — structured or plain text.

  - icon: 🔔
    title: Threshold Alerting + Webhooks
    details: Define rules like cpu.usage_pct > 80 for 5m. Fires a webhook POST to Slack, Discord, Teams, or any HTTP endpoint on FIRING and RESOLVED transitions.

  - icon: 🖥️
    title: Live Dashboard
    details: Dark-theme dashboard with SVG sparklines, trend indicators, animated alert badges, and host health status. Updates every 5 seconds without page reload.

  - icon: 🩺
    title: Host Health
    details: Tracks last heartbeat per host. Marks hosts as stale (>20s) or down (>50s) and fires a synthetic host.down alert with webhook notification.

  - icon: 🗃️
    title: TimescaleDB Storage
    details: Metrics in a hypertable with time-bucket query API. Automatic retention (30d metrics / 14d logs) and compression (7d) via TimescaleDB background workers.

  - icon: 🔑
    title: JWT Authentication
    details: Shared HS256 secret. Agent auto-mints 24h tokens — no manual key rotation. The TypeScript SDK does the same for Node.js apps.

  - icon: 🐳
    title: Docker-ready
    details: Multi-stage Dockerfiles for agent and server. One docker compose up --build and everything is running.
---

## What it looks like

```
┌─ MiniObserv ──────────────────── ● live  🔴 1 firing ──────┐
├─ HOSTS ────┬────────────────────────────────────────────────┤
│            │  ⚠ Memory Used > 8  |  actual: 10.36%          │
│ ● web-01   │                                                │
│ ● web-02   │  ┌──────────────┐  ┌──────────────┐           │
│ ◐ staging  │  │ CPU Usage    │  │ Memory Used  │           │
│ ✕ worker   │  │   0.70%      │  │   10.19%     │           │
│            │  │ → stable     │  │ ↑ rising     │           │
│            │  │ ▁▂▁▁▂▁▁▁    │  │ ▃▄▅▅▄▅▅▄▅   │           │
│            │  └──────────────┘  └──────────────┘           │
│            │                                                │
│            │  Logs (20)                                     │
│            │  16:42:31  INFO   GET /tasks → 200 (0ms)       │
│            │  16:42:31  ERROR  DB connection timeout        │
└────────────┴────────────────────────────────────────────────┘
```

Host indicators: **●** ok · **◐** stale · **✕** down

## 5-minute setup

```bash
git clone https://github.com/KamerrEzz/theminidog.git
cd theminidog/deployments
docker compose up --build
```

Open **http://localhost:8080** — dashboard is live as soon as the agent starts collecting.

## Alert notifications

```bash
# Fire to Slack when any metric exceeds its threshold
ALERT_RULES='[{"host":"*","name":"cpu.usage_pct","op":">","threshold":80,"for":"5m"}]'
ALERT_NOTIFICATIONS='[{"type":"webhook","url":"https://hooks.slack.com/services/YOUR/WEBHOOK"}]'
```

Any HTTP webhook works — Slack, Discord, Teams, PagerDuty, or your own endpoint.
