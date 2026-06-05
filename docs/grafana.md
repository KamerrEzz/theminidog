# Grafana Integration

MiniObserv exposes a Prometheus-compatible `/metrics` endpoint. Any tool that scrapes Prometheus — Grafana, Prometheus itself, Datadog agent, etc. — can read MiniObserv data.

## Endpoint

`GET /metrics` — public, no authentication required.

Response format: Prometheus text format 0.0.4

Example output:
```
# HELP miniobserv_cpu_usage_pct MiniObserv metric: cpu.usage_pct
# TYPE miniobserv_cpu_usage_pct gauge
miniobserv_cpu_usage_pct{host="web-01"} 42.5 1717600943000
miniobserv_cpu_usage_pct{host="web-02"} 71.2 1717600943000

# HELP miniobserv_mem_used_pct MiniObserv metric: mem.used_pct
# TYPE miniobserv_mem_used_pct gauge
miniobserv_mem_used_pct{host="web-01"} 68.4 1717600943000
```

## Metric names

Dots are replaced with underscores and prefixed with `miniobserv_`:

| MiniObserv name | Prometheus name |
|---|---|
| `cpu.usage_pct` | `miniobserv_cpu_usage_pct` |
| `mem.used_pct` | `miniobserv_mem_used_pct` |
| `mem.used_bytes` | `miniobserv_mem_used_bytes` |
| `disk.used_pct` | `miniobserv_disk_used_pct` |
| `net.bytes_in` | `miniobserv_net_bytes_in` |
| `net.bytes_out` | `miniobserv_net_bytes_out` |

## Quick start with Grafana

Add to your existing `docker-compose.yml`:

```bash
# From the deployments/ directory
docker compose -f docker-compose.yml -f grafana/docker-compose.yml up
```

- **Grafana**: http://localhost:3000 (admin / admin)
- **Prometheus**: http://localhost:9090

Prometheus is pre-configured to scrape MiniObserv every 15 seconds. Grafana has Prometheus as its default data source.

## Manual Prometheus config

If you're running your own Prometheus:

```yaml
scrape_configs:
  - job_name: miniobserv
    static_configs:
      - targets: ['your-miniobserv-host:8080']
    metrics_path: /metrics
    scrape_interval: 15s
```
