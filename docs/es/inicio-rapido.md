# Inicio rápido — MiniObserv

Esta guía le lleva desde cero hasta un sistema de observabilidad en funcionamiento en menos de diez minutos.

---

## 1. Requisitos previos

| Herramienta | Versión mínima | Propósito |
|-------------|---------------|-----------|
| Go | 1.23+ | Compilar los binarios |
| Docker | 24+ | Ejecutar TimescaleDB y los servicios |
| Docker Compose | v2 (`docker compose`) | Orquestar el stack completo |
| make | cualquiera | Atajos de compilación opcionales |

Verifique su entorno:

```bash
go version          # go1.23.x o superior
docker compose version
```

---

## 2. Construir los binarios

Desde la raíz del repositorio:

```bash
# Agente de recolección
go build -o bin/agent ./cmd/agent

# Servidor de ingesta y consulta
go build -o bin/server ./cmd/server
```

Ambos binarios quedan en `bin/`. También puede usar el Makefile si está disponible:

```bash
make build
```

---

## 3. Configuración

### Variables del agente

| Variable | Obligatoria | Predeterminado | Descripción |
|----------|-------------|----------------|-------------|
| `SERVER_URL` | sí | — | URL HTTP/HTTPS del servidor, p. ej. `http://localhost:8080` |
| `AGENT_TOKEN` | recomendada | vacío | Secreto compartido HS256 para firmar los JWT |
| `AGENT_HOST` | no | hostname del sistema | Nombre lógico del host que aparecerá en las métricas |
| `COLLECT_INTERVAL` | no | `10s` | Intervalo de recolección (1 s – 300 s) |
| `LOG_LEVEL` | no | `info` | Nivel de log: `debug`, `info`, `warn`, `error` |
| `LOG_PATHS` | no | vacío | Rutas de archivos de log a vigilar (separadas por coma) |

### Variables del servidor

| Variable | Obligatoria | Predeterminado | Descripción |
|----------|-------------|----------------|-------------|
| `DATABASE_URL` | sí | — | DSN de PostgreSQL, p. ej. `postgres://usuario:clave@host:5432/bd?sslmode=disable` |
| `AGENT_TOKEN` | sí | — | Secreto compartido HS256, mínimo 16 caracteres |
| `LISTEN_ADDR` | no | `:8080` | Dirección y puerto de escucha |
| `MIGRATIONS_PATH` | no | `./migrations` | Directorio con los archivos de migración SQL |
| `LOG_LEVEL` | no | `info` | Nivel de log: `debug`, `info`, `warn`, `error` |
| `REQUEST_TIMEOUT` | no | `10s` | Tiempo máximo por petición HTTP (1 s – 120 s) |
| `SHUTDOWN_TIMEOUT` | no | `5s` | Tiempo de espera para apagado limpio (1 s – 30 s) |

> **Importante:** `DATABASE_URL` debe usar el esquema `postgres://` o `postgresql://`. El esquema `pgx5://` es solo para el driver de migraciones interno.

---

## 4. Ejecutar con Docker Compose

El archivo `deployments/docker-compose.yml` levanta TimescaleDB, el servidor y el agente en una sola red interna.

```bash
cd deployments
docker compose up --build
```

Qué esperar en los logs al arrancar:

```
timescaledb  | database system is ready to accept connections
server       | migrations applied successfully
server       | listening on :8080
agent        | starting collection loop interval=10s
agent        | batch sent metrics=7
```

- Los primeros ticks del agente no incluirán métricas de red (`net.*`) — esto es normal (ver sección 8).
- El servidor aplica las migraciones automáticamente al arrancar; no es necesario ejecutarlas a mano.

---

## 5. Generar AGENT_TOKEN

### Crear un secreto fuerte

```bash
# openssl (disponible en Linux, macOS y WSL)
openssl rand -base64 32

# Ejemplo de salida:
# 7kQzP1mXwN8vLhR3tYsA0bJdCeOuGfIi4nKpMqVrTg==
```

El secreto debe tener al menos 16 caracteres. Úselo como valor de `AGENT_TOKEN` tanto en el servidor como en el agente.

### Generar un JWT manualmente (para pruebas)

Si necesita llamar a la API directamente sin ejecutar el agente:

```bash
# Instalar jwt-cli (opcional, solo para pruebas)
# go install github.com/nicholasgasior/jwt-cli@latest

# O usar Python si está disponible:
python3 - <<'EOF'
import hmac, hashlib, base64, json, time

secret = b"TU_SECRETO_AQUI"
header = base64.urlsafe_b64encode(json.dumps({"alg":"HS256","typ":"JWT"}).encode()).rstrip(b"=")
payload = base64.urlsafe_b64encode(json.dumps({"sub":"agent","exp": int(time.time()) + 86400}).encode()).rstrip(b"=")
sig_input = header + b"." + payload
sig = base64.urlsafe_b64encode(hmac.new(secret, sig_input, hashlib.sha256).digest()).rstrip(b"=")
print((sig_input + b"." + sig).decode())
EOF
```

> El agente genera sus propios JWTs automáticamente si `AGENT_TOKEN` está configurado. El script anterior es solo para pruebas manuales.

---

## 6. Verificar que funciona

Espere al menos 30 segundos para que el agente envíe su primer batch completo (con deltas de red).

### Verificar liveness

```bash
curl -s http://localhost:8080/healthz
# ok
```

### Verificar readiness (conexión con la base de datos)

```bash
curl -s http://localhost:8080/readyz
# ok
```

### Obtener un token JWT para las pruebas

Si está usando Docker Compose con el valor de ejemplo, el token es `change-me-use-a-real-secret-min-16ch`. Para producción, reemplácelo y genere un JWT como se describe en la sección 5.

### Consultar métricas de CPU (últimos 5 minutos)

```bash
TOKEN="TU_JWT_AQUI"
HOST="TU_HOST_AQUI"

curl -s \
  -H "Authorization: Bearer $TOKEN" \
  "http://localhost:8080/api/v1/metrics/query?host=$HOST&name=cpu.usage_pct&from=2026-06-05T12:00:00Z&to=2026-06-05T12:05:00Z&bucket=1m&agg=avg"
```

Respuesta esperada:

```json
{
  "host": "mi-servidor",
  "name": "cpu.usage_pct",
  "bucket": "1m",
  "agg": "avg",
  "points": [
    { "time": "2026-06-05T12:04:00Z", "value": 12.5 },
    { "time": "2026-06-05T12:03:00Z", "value": 11.8 }
  ]
}
```

---

## 7. Detener el sistema

Presione `Ctrl+C` o envíe `SIGINT` al proceso. El servidor acepta la señal y espera a que las peticiones en vuelo terminen antes de cerrarse (según `SHUTDOWN_TIMEOUT`, predeterminado 5 s).

```bash
# Con Docker Compose:
docker compose down

# Para eliminar también los volúmenes de datos:
docker compose down -v
```

---

## 8. Solución de problemas

### El servidor no arranca: "DATABASE_URL must be a valid postgres:// URL"

El DSN debe comenzar con `postgres://` o `postgresql://`, no con `pgx5://` ni `postgresql5://`.

```bash
# Correcto:
DATABASE_URL="postgres://usuario:clave@localhost:5432/miniobserv?sslmode=disable"

# Incorrecto:
DATABASE_URL="pgx5://usuario:clave@localhost:5432/miniobserv"
```

### El servidor no arranca: "AGENT_TOKEN must be at least 16 characters"

El secreto configurado tiene menos de 16 caracteres. Genere uno nuevo:

```bash
openssl rand -base64 32
```

### El agente no envía métricas de red en el primer tick

Esto es por diseño. `net.bytes_in` y `net.bytes_out` usan **semántica de deltas**: el primer tick solo registra el estado inicial de los contadores. A partir del segundo tick, las métricas de red estarán disponibles normalmente.

### Las consultas devuelven `[]` aunque el agente está enviando

Verifique que los parámetros `host` y `from`/`to` coincidan exactamente con los datos almacenados. Puede inspeccionar el host real en los logs del agente:

```bash
docker compose logs agent | grep "batch sent"
```

### El agente no puede conectarse al servidor: "connection refused"

- Confirme que `SERVER_URL` apunta a la dirección correcta.
- Si ejecuta fuera de Docker, use `http://localhost:8080` en lugar de `http://server:8080`.

### Error 401 Unauthorized en la API

- Confirme que `AGENT_TOKEN` es idéntico en el servidor y en el agente.
- Verifique que el JWT no haya expirado (duración predeterminada: 24 horas).
- El encabezado debe ser exactamente `Authorization: Bearer <token>`.
