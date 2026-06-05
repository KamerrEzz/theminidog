# Integración con Grafana

MiniObserv expone un endpoint `/metrics` compatible con Prometheus. Cualquier herramienta que lea Prometheus — Grafana, el propio Prometheus, el agente de Datadog, etc. — puede consumir los datos de MiniObserv.

## Endpoint

`GET /metrics` — público, no requiere autenticación.

Formato de respuesta: Prometheus text format 0.0.4

Ejemplo de salida:
```
# HELP miniobserv_cpu_usage_pct MiniObserv metric: cpu.usage_pct
# TYPE miniobserv_cpu_usage_pct gauge
miniobserv_cpu_usage_pct{host="web-01"} 42.5 1717600943000
miniobserv_cpu_usage_pct{host="web-02"} 71.2 1717600943000

# HELP miniobserv_mem_used_pct MiniObserv metric: mem.used_pct
# TYPE miniobserv_mem_used_pct gauge
miniobserv_mem_used_pct{host="web-01"} 68.4 1717600943000
```

## Nombres de métricas

Los puntos se reemplazan por guiones bajos y el resultado se prefija con `miniobserv_`:

| Nombre en MiniObserv | Nombre en Prometheus |
|---|---|
| `cpu.usage_pct` | `miniobserv_cpu_usage_pct` |
| `mem.used_pct` | `miniobserv_mem_used_pct` |
| `mem.used_bytes` | `miniobserv_mem_used_bytes` |
| `disk.used_pct` | `miniobserv_disk_used_pct` |
| `net.bytes_in` | `miniobserv_net_bytes_in` |
| `net.bytes_out` | `miniobserv_net_bytes_out` |

## Inicio rápido con Grafana

Agrega esto a tu `docker-compose.yml` existente:

```bash
# Desde el directorio deployments/
docker compose -f docker-compose.yml -f grafana/docker-compose.yml up
```

- **Grafana**: http://localhost:3000 (admin / admin)
- **Prometheus**: http://localhost:9090

Prometheus está preconfigurado para hacer scraping de MiniObserv cada 15 segundos. Grafana tiene Prometheus como fuente de datos predeterminada.

## Configuración manual de Prometheus

Si ya tienes tu propio Prometheus:

```yaml
scrape_configs:
  - job_name: miniobserv
    static_configs:
      - targets: ['tu-host-miniobserv:8080']
    metrics_path: /metrics
    scrape_interval: 15s
```
