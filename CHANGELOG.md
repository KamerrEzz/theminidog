# Changelog

All notable changes to this project are documented in this file.
Format based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [1.0.0] - 2026-06-05

### Added — Week 5: Notifications, Host Health, Retention
- Webhook notifications on FIRING/RESOLVED alert transitions (`ALERT_NOTIFICATIONS` env var)
- Multi-host health tracking: ok/stale/down status with heartbeat detection
- `GET /api/v1/hosts` endpoint (public) returning per-host health status
- Dashboard sidebar color-coded dots: green (ok), orange (stale), red (down)
- Synthetic `host.down` alert fires when a host exceeds `HOST_DOWN_AFTER`
- TimescaleDB retention policies: metrics 30d, logs 14d
- TimescaleDB compression policies: metrics and logs after 7d
- Migration 003: converts `logs` table to hypertable
- `HOST_STALE_AFTER` and `HOST_DOWN_AFTER` env vars (defaults: 20s, 50s)
- TypeScript SDK v0.2.0: `getAlerts()`, `getHosts()`, `queryLogs()` methods
- Docker Hub images: `kamerrezz/miniobserv-server`, `kamerrezz/miniobserv-agent`
- npm package: `@kamerrezz/miniobserv@0.2.0`
- VitePress documentation site at kamerrezz.github.io/theminidog/ (EN + ES)
- GitHub Actions: CI, Docker multi-platform build, npm publish, docs deploy

### Added — Week 4: Dashboard + Threshold Alerting
- Live dark-theme HTML dashboard with SVG sparklines (5s JS polling, no page reload)
- Threshold alerting engine: `>/<` rules on any metric, any host, with `for` duration
- In-memory alert evaluator with 30s ticker and FIRING/RESOLVED state machine
- `GET /api/v1/alerts` endpoint (public)
- `ALERT_RULES` env var: JSON array of threshold rule objects
- Human-readable metric labels on dashboard cards
- Trend indicators: ↑ rising / ↓ falling / → stable

### Added — Week 3: Logs Pipeline
- Agent tails arbitrary log files with rotation detection (fsnotify)
- `POST /api/v1/logs` ingest endpoint (JWT)
- `GET /api/v1/logs/query` filtered, keyset-paginated log search
- Log storage: plain PostgreSQL table with BIGSERIAL cursor
- Migration 002: logs table schema
- `LOG_PATHS` env var for comma-separated file paths

### Added — Week 2: Server + Storage + Query API
- Server binary with chi router and JWT HS256 middleware
- TimescaleDB hypertable for time-series metrics storage
- Bulk insert via `pgx.Batch` for high-throughput ingestion
- `POST /api/v1/metrics` ingest endpoint
- `GET /api/v1/metrics/query` time-bucket query API
- Auto-migrations on startup via golang-migrate
- Migration 001: metrics hypertable
- `GET /healthz` and `GET /readyz` endpoints
- TypeScript SDK v0.1.0: `pushMetric()`, `pushMetrics()`, `queryMetrics()`

### Added — Week 1: Agent
- Go agent binary collecting 9 system metrics every 10s
- Collectors: CPU (delta from /proc/stat), memory, disk, network (delta)
- `statFn` injection pattern for deterministic unit tests
- HTTP sender with exponential backoff and JWT minting
- `COLLECT_INTERVAL`, `AGENT_HOST`, `SERVER_URL`, `AGENT_TOKEN` env vars

## [0.1.0] - 2026-04-01

Initial project setup. Module: `github.com/kamerrezz/theminidog`.
