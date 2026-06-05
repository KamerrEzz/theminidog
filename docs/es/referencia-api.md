# Referencia de API — MiniObserv

Base URL: `http://<host>:<puerto>` (predeterminado `:8080`)

---

## 1. Autenticación

Las rutas de métricas requieren un token JWT en el encabezado `Authorization`:

```
Authorization: Bearer <jwt>
```

**Cómo funciona:**

1. El secreto `AGENT_TOKEN` (mínimo 16 caracteres) es compartido entre el agente y el servidor.
2. El agente firma un JWT HS256 con `exp = now + 24h` usando ese secreto.
3. El servidor valida la firma, el algoritmo (solo `HS256` es aceptado) y la expiración en cada petición.
4. Si el token es inválido, expirado o ausente, el servidor devuelve `401 Unauthorized`.

Las rutas `/healthz` y `/readyz` no requieren autenticación.

---

## 2. POST /api/v1/metrics

Ingesta un batch de métricas recolectadas por el agente.

### Autenticación

Requerida (Bearer JWT).

### Cuerpo de la petición

`Content-Type: application/json`

```json
{
  "host": "string",
  "metrics": [
    {
      "time":   "RFC3339",
      "host":   "string",
      "name":   "string",
      "value":  "number",
      "labels": { "clave": "valor" }
    }
  ]
}
```

| Campo | Tipo | Obligatorio | Descripción |
|-------|------|-------------|-------------|
| `host` | string | sí | Nombre lógico del host origen del batch |
| `metrics` | array | sí | Lista de mediciones (1 – 1000 elementos) |
| `metrics[].time` | RFC3339 | sí | Instante de la medición (con zona horaria) |
| `metrics[].host` | string | sí | Nombre del host (debe coincidir con el campo raíz) |
| `metrics[].name` | string | sí | Nombre canónico de la métrica (ver sección 6) |
| `metrics[].value` | float64 | sí | Valor numérico finito (no NaN, no ±Inf) |
| `metrics[].labels` | object | no | Metadatos adicionales como pares clave-valor |

### Respuesta exitosa

**HTTP 202 Accepted**

```json
{ "ingested": 7 }
```

### Ejemplo

```bash
TOKEN="TU_JWT_AQUI"

curl -s -X POST http://localhost:8080/api/v1/metrics \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "host": "servidor-prod-01",
    "metrics": [
      {
        "time": "2026-06-05T12:00:00Z",
        "host": "servidor-prod-01",
        "name": "cpu.usage_pct",
        "value": 45.2,
        "labels": { "core": "total" }
      },
      {
        "time": "2026-06-05T12:00:00Z",
        "host": "servidor-prod-01",
        "name": "mem.used_pct",
        "value": 68.1
      }
    ]
  }'
```

Respuesta:

```json
{ "ingested": 2 }
```

### Validaciones

- `metrics` no puede estar vacío.
- El batch no puede superar **1000 métricas**.
- Cada `name` debe ser uno de los nueve nombres canónicos (ver sección 6).
- `time` no puede ser el instante cero.
- `value` debe ser un número finito.
- Las claves y valores de `labels` no pueden estar vacíos.

---

## 3. GET /api/v1/metrics/query

Consulta una serie temporal con agregación por bucket de tiempo.

### Autenticación

Requerida (Bearer JWT).

### Parámetros de consulta

| Parámetro | Tipo | Obligatorio | Predeterminado | Descripción |
|-----------|------|-------------|----------------|-------------|
| `host` | string | sí | — | Nombre exacto del host a consultar |
| `name` | string | sí | — | Nombre canónico de la métrica (ver sección 6) |
| `from` | RFC3339 | sí | — | Inicio del rango temporal (inclusive) |
| `to` | RFC3339 | sí | — | Fin del rango temporal (inclusive) |
| `bucket` | string | no | `1m` | Granularidad del bucket: `1m`, `5m`, `15m`, `1h`, `1d` |
| `agg` | string | no | `avg` | Función de agregación: `avg`, `max`, `min` |

### Respuesta exitosa

**HTTP 200 OK**

```json
{
  "host":   "servidor-prod-01",
  "name":   "cpu.usage_pct",
  "bucket": "1m",
  "agg":    "avg",
  "points": [
    { "time": "2026-06-05T12:04:00Z", "value": 45.2 },
    { "time": "2026-06-05T12:03:00Z", "value": 42.8 },
    { "time": "2026-06-05T12:02:00Z", "value": 39.1 }
  ]
}
```

Los puntos se devuelven en orden **descendente** (más reciente primero). Si no hay datos en el rango, `points` es `[]`.

### Ejemplos

**Uso de CPU en los últimos 5 minutos, bucket de 1 minuto:**

```bash
TOKEN="TU_JWT_AQUI"

curl -s \
  -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/metrics/query?host=servidor-prod-01&name=cpu.usage_pct&from=2026-06-05T12:00:00Z&to=2026-06-05T12:05:00Z&bucket=1m&agg=avg"
```

**Uso de memoria máximo en la última hora:**

```bash
curl -s \
  -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/metrics/query?host=servidor-prod-01&name=mem.used_pct&from=2026-06-05T11:00:00Z&to=2026-06-05T12:00:00Z&bucket=1h&agg=max"
```

**Bytes de red recibidos, bucket de 5 minutos:**

```bash
curl -s \
  -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/metrics/query?host=servidor-prod-01&name=net.bytes_in&from=2026-06-05T11:00:00Z&to=2026-06-05T12:00:00Z&bucket=5m&agg=avg"
```

### Restricciones

- El rango `from` → `to` no puede superar **30 días**.
- `from` debe ser anterior a `to`.
- `bucket` debe ser uno de: `1m`, `5m`, `15m`, `1h`, `1d`.
- `agg` debe ser uno de: `avg`, `max`, `min`.

---

## 4. GET /healthz

Sonda de liveness. Verifica que el proceso del servidor está en ejecución.

### Autenticación

No requerida.

### Respuesta

**HTTP 200 OK** — siempre, sin consultar la base de datos.

```
ok
```

### Cuándo usarla

- Configuración de sondas de liveness en Kubernetes (`livenessProbe`).
- Verificación rápida de que el proceso arrancó correctamente.
- No indica si la base de datos está disponible (para eso, use `/readyz`).

### Ejemplo

```bash
curl -s http://localhost:8080/healthz
# ok
```

---

## 5. GET /readyz

Sonda de readiness. Verifica que el servidor puede atender peticiones y que la conexión con la base de datos funciona.

### Autenticación

No requerida.

### Respuesta exitosa

**HTTP 200 OK**

```
ok
```

### Respuesta en caso de error

**HTTP 503 Service Unavailable**

```json
{ "error": "db unavailable" }
```

### Cuándo usarla

- Configuración de sondas de readiness en Kubernetes (`readinessProbe`).
- Verificación post-arranque antes de dirigir tráfico al servidor.
- Diagnóstico de problemas de conectividad con TimescaleDB.

### Ejemplo

```bash
curl -s http://localhost:8080/readyz
# ok

# Verificar el código de estado HTTP:
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/readyz
# 200
```

---

## 6. Respuestas de error

Todas las respuestas de error siguen este formato JSON:

```json
{ "error": "descripción del error" }
```

### Códigos de estado

| Código | Significado | Causas típicas |
|--------|-------------|----------------|
| `400 Bad Request` | Petición mal formada | JSON inválido, campo faltante, nombre de métrica desconocido, batch vacío, batch > 1000, parámetro de query inválido, rango > 30 días |
| `401 Unauthorized` | Token ausente o inválido | Sin encabezado `Authorization`, token expirado, firma incorrecta, algoritmo distinto de HS256 |
| `500 Internal Server Error` | Error interno del servidor | Fallo en la base de datos al insertar o consultar |
| `503 Service Unavailable` | Base de datos no disponible | Solo en `/readyz`; la base de datos no responde al ping |

### Ejemplos de respuestas de error

**400 — Nombre de métrica desconocido:**

```json
{ "error": "metric[0]: unknown metric name \"cpu.load\"" }
```

**400 — Batch excede el límite:**

```json
{ "error": "batch exceeds maximum size of 1000" }
```

**400 — Parámetro de bucket inválido:**

```json
{ "error": "invalid bucket \"2m\": must be one of 1m,5m,15m,1h,1d" }
```

**401 — Token ausente:**

```json
{ "error": "unauthorized" }
```

**400 — Rango temporal inválido:**

```json
{ "error": "time range must not exceed 30 days" }
```

---

## 7. Referencia de métricas

### Tabla de nombres canónicos

| Nombre | Unidad | Labels | Emitido desde |
|--------|--------|--------|---------------|
| `cpu.usage_pct` | % (0–100) | `core=total\|0\|1\|…` | Primer tick |
| `mem.used_pct` | % (0–100) | — | Primer tick |
| `mem.used_bytes` | bytes | — | Primer tick |
| `mem.total_bytes` | bytes | — | Primer tick |
| `disk.used_pct` | % (0–100) | `mount=/` | Primer tick |
| `disk.used_bytes` | bytes | `mount=/` | Primer tick |
| `disk.total_bytes` | bytes | `mount=/` | Primer tick |
| `net.bytes_in` | bytes (delta) | `iface=eth0` | **Segundo tick** |
| `net.bytes_out` | bytes (delta) | `iface=eth0` | **Segundo tick** |

### Notas sobre labels

- **`cpu.usage_pct`**: `core=total` representa el promedio de todos los núcleos; `core=0`, `core=1`, etc. representan cada núcleo físico.
- **`disk.*`**: `mount` contiene el punto de montaje exacto del sistema de archivos (p. ej. `/`, `/data`, `/home`).
- **`net.*`**: `iface` contiene el nombre de la interfaz de red (p. ej. `eth0`, `ens3`, `enp3s0`). El loopback (`lo`) siempre se excluye.
- Las métricas de memoria no tienen labels porque representan el estado global del sistema.

### Semántica de net.bytes_in / net.bytes_out

Estos valores representan el **delta de bytes** desde el tick anterior, no el total acumulado. Por eso no se emiten en el primer tick del agente: el primer tick solo registra el estado inicial de los contadores del sistema operativo como referencia para el cálculo siguiente.

---

## 8. Límites

| Límite | Valor |
|--------|-------|
| Tamaño máximo de batch (POST /api/v1/metrics) | 1000 métricas |
| Rango máximo de consulta (GET /api/v1/metrics/query) | 30 días |
| Granularidades de bucket disponibles | `1m`, `5m`, `15m`, `1h`, `1d` |
| Funciones de agregación disponibles | `avg`, `max`, `min` |
| Duración del JWT generado por el agente | 24 horas |
| Longitud mínima de AGENT_TOKEN | 16 caracteres |
| Algoritmo JWT aceptado | HS256 únicamente |
