# Arquitectura — MiniObserv

---

## 1. Visión general

MiniObserv es una plataforma de observabilidad de infraestructura **minimalista y autoalojada**. Su objetivo es responder a una pregunta concreta: _¿qué está haciendo este servidor ahora y en los últimos N minutos?_

**Lo que es:**
- Un sistema de recolección y almacenamiento de métricas de sistema (CPU, memoria, disco, red).
- Una API de consulta con agregaciones temporales basada en TimescaleDB.
- Una solución operativa lista para ejecutarse con `docker compose up`.

**Lo que no es:**
- No es un sistema de alertas ni de visualización (no incluye Grafana ni similares).
- No es una plataforma de trazabilidad distribuida ni de logs estructurados.
- No está diseñado para escalado horizontal multi-instancia de servidor en esta versión.

---

## 2. Diagrama de componentes

```
┌────────────────────────────────────────────────────────────────┐
│  Host monitorizado                                             │
│                                                                │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │  Agente (cmd/agent)                                      │  │
│  │                                                          │  │
│  │  ┌──────────────┐    ┌────────────────────────────────┐  │  │
│  │  │  Collectors  │───▶│  Agent (goroutines)            │  │  │
│  │  │  cpu         │    │  collectLoop → batches chan    │  │  │
│  │  │  memory      │    │  senderLoop  ← batches chan    │  │  │
│  │  │  disk        │    └──────────────┬─────────────────┘  │  │
│  │  │  network     │                   │                     │  │
│  │  └──────────────┘                   │ HTTP POST           │  │
│  └──────────────────────────────────── │ ────────────────────┘  │
│                                        │ JWT Bearer             │
└───────────────────────────────────────────────────────────────-┘
                                         │
                                         ▼
┌────────────────────────────────────────────────────────────────┐
│  Servidor (cmd/server)                                         │
│                                                                │
│  ┌──────────────┐   ┌────────────────┐   ┌────────────────┐   │
│  │  JWTMiddle-  │──▶│  api.Handle-   │──▶│  storage.      │   │
│  │  ware        │   │  Ingest /      │   │  pgxMetric-    │   │
│  │              │   │  HandleQuery   │   │  Repository    │   │
│  └──────────────┘   └────────────────┘   └───────┬────────┘   │
│                                                   │            │
│  GET /healthz  ──▶  HandleHealthz (sin auth)      │ pgx.Batch  │
│  GET /readyz   ──▶  HandleReadyz  (sin auth)      │ pgxpool    │
└──────────────────────────────────────────────────────────────┘
                                                    │
                                                    ▼
                              ┌─────────────────────────────────┐
                              │  TimescaleDB (PostgreSQL 16)     │
                              │                                  │
                              │  tabla: metrics                  │
                              │  hypertable por time             │
                              │  chunk: 1 día                    │
                              │  índice: (host, name, time DESC) │
                              └─────────────────────────────────┘
```

---

## 3. Flujo de datos

```
1. Ticker (COLLECT_INTERVAL)
        │
        ▼
2. collector.Registry.CollectAll()
   ├── CPUCollector     → cpu.usage_pct (por núcleo y total)
   ├── MemoryCollector  → mem.used_pct, mem.used_bytes, mem.total_bytes
   ├── DiskCollector    → disk.used_pct, disk.used_bytes, disk.total_bytes
   └── NetworkCollector → net.bytes_in, net.bytes_out (delta desde tick anterior)
        │
        ▼
3. []model.Metric → model.MetricBatch{Host, Metrics}
        │
        ▼
4. batches chan (buffer 10) — desacopla recolección de envío
        │
        ▼
5. HTTPSender.Send() — POST /api/v1/metrics con Bearer JWT
   └── backoff exponencial en errores transitorios (1 s → 60 s, ±25 % jitter)
        │
        ▼
6. JWTMiddleware valida el token HS256
        │
        ▼
7. api.HandleIngest() — decodifica JSON, valida, límite 1000 métricas
        │
        ▼
8. storage.Insert() — pgx.Batch, un round-trip por batch
        │
        ▼
9. TimescaleDB — INSERT INTO metrics (time, host, name, value, labels)
        │
        ▼
10. GET /api/v1/metrics/query → time_bucket() + avg/max/min → []QueryPoint
```

---

## 4. Estructura de paquetes

```
github.com/kamerrezz/theminidog/
├── cmd/
│   ├── agent/          — punto de entrada del agente: carga config, construye
│   │                     el grafo de dependencias, arranca Agent.Run()
│   └── server/         — punto de entrada del servidor: carga config, aplica
│                         migraciones, arranca el servidor HTTP
├── internal/
│   ├── agent/
│   │   ├── agent.go    — coordinación: collectLoop + senderLoop con goroutines
│   │   ├── collector/
│   │   │   ├── collector.go  — interfaz Collector y Registry
│   │   │   ├── cpu.go        — uso de CPU por núcleo (gopsutil)
│   │   │   ├── memory.go     — uso de RAM
│   │   │   ├── disk.go       — uso de disco por punto de montaje
│   │   │   └── network.go    — bytes in/out por interfaz (delta semántico)
│   │   └── sender/
│   │       └── sender.go     — HTTPSender con backoff exponencial y JWT
│   ├── config/
│   │   ├── agent.go    — LoadAgent(): variables de entorno del agente
│   │   └── server.go   — LoadServerConfig(): variables de entorno del servidor
│   ├── model/
│   │   └── metric.go   — Metric, MetricBatch, validación y lista blanca de nombres
│   └── server/
│       ├── server.go   — ciclo de vida HTTP: arranque, graceful shutdown
│       ├── api/
│       │   ├── router.go     — registro de rutas
│       │   ├── metrics.go    — HandleIngest, HandleQuery
│       │   ├── health.go     — HandleHealthz, HandleReadyz
│       │   ├── middleware.go — JWTMiddleware HS256
│       │   └── errors.go     — writeError: formato JSON estándar
│       └── storage/
│           └── metrics.go    — pgxMetricRepository: Insert (batch) y Query
├── migrations/
│   └── 001_create_metrics.up.sql   — extensión TimescaleDB, tabla, hypertable, índice
└── deployments/
    └── docker-compose.yml          — stack completo: TimescaleDB + servidor + agente
```

---

## 5. Diseño del almacenamiento

### Por qué TimescaleDB

TimescaleDB extiende PostgreSQL con hypertables: tablas particionadas automáticamente por tiempo. Esto permite:

- **Consultas eficientes por rango temporal** sin necesidad de particionamiento manual.
- **`time_bucket()`**: agrupación temporal nativa con granularidades arbitrarias.
- **Retención de datos** configurable mediante políticas (no implementada en esta versión, pero disponible).
- Compatibilidad total con el ecosistema PostgreSQL (pgx, migraciones, JSON, índices).

### Modelo estrecho

La tabla tiene exactamente cinco columnas:

```sql
CREATE TABLE metrics (
    time   TIMESTAMPTZ      NOT NULL,
    host   TEXT             NOT NULL,
    name   TEXT             NOT NULL,
    value  DOUBLE PRECISION NOT NULL,
    labels JSONB
);
```

Este diseño "estrecho" tiene ventajas deliberadas:

- **Esquema fijo**: no se requieren migraciones al añadir nuevas métricas; solo se cambia el código del agente.
- **Labels JSONB**: permite metadatos variables por tipo de métrica (`core=0`, `mount=/`, `iface=eth0`) sin columnas adicionales.
- **Una fila por medición**: facilita el razonamiento sobre los datos y simplifica las consultas.

### Hypertable

```sql
SELECT create_hypertable('metrics', 'time',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);
```

Cada día de datos se almacena en un chunk independiente. Las consultas por rango temporal solo tocan los chunks relevantes, lo que reduce drásticamente el I/O.

### Índice compuesto

```sql
CREATE INDEX idx_metrics_host_name_time ON metrics (host, name, time DESC);
```

El patrón de consulta habitual es `WHERE host = $1 AND name = $2 AND time BETWEEN $3 AND $4`. El índice cubre este acceso en orden descendente, ideal para consultas de "los últimos N minutos".

### Inserción en batch

El repositorio usa `pgx.Batch` para enviar todas las métricas de un tick en un único round-trip de red. Es obligatorio llamar a `defer br.Close()` para liberar la conexión al pool.

---

## 6. Flujo de autenticación

```
AGENT_TOKEN (secreto compartido, ≥16 chars)
        │
        ├── Agente: genera JWT HS256 con exp=now+24h
        │         Header: {"alg":"HS256","typ":"JWT"}
        │         Payload: {"sub":"agent","exp":<unix>}
        │         Firma: HMAC-SHA256(header.payload, AGENT_TOKEN)
        │
        └── Servidor: JWTMiddleware valida en cada petición autenticada
                      ├── Verifica firma con AGENT_TOKEN
                      ├── Verifica algoritmo == HS256 (bloquea alg=none y RS256)
                      └── Verifica expiración automáticamente
```

Las rutas `/healthz` y `/readyz` no requieren autenticación (sondas de infraestructura).

---

## 7. Convención de nombres de métricas

MiniObserv usa un conjunto cerrado de nueve nombres canónicos. El servidor rechaza cualquier nombre fuera de esta lista con HTTP 400.

| Nombre | Tipo | Labels | Descripción |
|--------|------|--------|-------------|
| `cpu.usage_pct` | porcentaje (0–100) | `core=total\|0\|1\|…` | Uso de CPU por núcleo y total |
| `mem.used_pct` | porcentaje (0–100) | — | Porcentaje de RAM usada |
| `mem.used_bytes` | bytes | — | Bytes de RAM en uso |
| `mem.total_bytes` | bytes | — | Bytes de RAM total |
| `disk.used_pct` | porcentaje (0–100) | `mount=/` | Uso de disco por punto de montaje |
| `disk.used_bytes` | bytes | `mount=/` | Bytes de disco usados |
| `disk.total_bytes` | bytes | `mount=/` | Bytes de disco total |
| `net.bytes_in` | bytes (delta) | `iface=eth0` | Bytes recibidos desde el tick anterior |
| `net.bytes_out` | bytes (delta) | `iface=eth0` | Bytes enviados desde el tick anterior |

---

## 8. Semántica de deltas en red

Los contadores del sistema operativo para `net.bytes_in` y `net.bytes_out` son **acumulativos**: siempre crecen desde el arranque. MiniObserv convierte estos contadores en **deltas por intervalo**:

```
tick N:   lee BytesRecv=1000 → guarda como prev; devuelve nil
tick N+1: lee BytesRecv=1150 → delta = 1150 - 1000 = 150 bytes → emite net.bytes_in{iface=eth0}=150
```

**Consecuencias:**

- El primer tick del agente devuelve cero métricas de red. Esto es correcto.
- Si una interfaz aparece por primera vez en el tick N+1 (no estaba en tick N), ese ciclo también se omite.
- Si el contador del sistema operativo retrocede (reinicio, overflow de 32 bits), el delta se clampea a cero.
- El loopback (`lo`) siempre se excluye.

---

## 9. Referencia de configuración

### Agente

| Variable | Obligatoria | Predeterminado | Validación |
|----------|-------------|----------------|-----------|
| `SERVER_URL` | sí | — | URL válida `http://` o `https://` |
| `AGENT_TOKEN` | no | vacío | Sin validación; vacío = sin autenticación JWT |
| `AGENT_HOST` | no | `os.Hostname()` | Cualquier string no vacío |
| `COLLECT_INTERVAL` | no | `10s` | Duración Go válida entre 1 s y 300 s |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_PATHS` | no | vacío | Rutas separadas por coma |

### Servidor

| Variable | Obligatoria | Predeterminado | Validación |
|----------|-------------|----------------|-----------|
| `DATABASE_URL` | sí | — | DSN `postgres://` o `postgresql://` válido |
| `AGENT_TOKEN` | sí | — | Mínimo 16 caracteres |
| `LISTEN_ADDR` | no | `:8080` | Dirección de escucha válida |
| `MIGRATIONS_PATH` | no | `./migrations` | Ruta al directorio de migraciones |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, `error` |
| `REQUEST_TIMEOUT` | no | `10s` | Entre 1 s y 120 s |
| `SHUTDOWN_TIMEOUT` | no | `5s` | Entre 1 s y 30 s |
