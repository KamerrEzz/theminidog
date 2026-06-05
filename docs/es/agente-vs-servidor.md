# Agente vs Servidor — ¿Por qué dos binarios?

Esta es una de las primeras preguntas que surgen al ver este proyecto. Hay dos binarios separados en Go: `agent` y `server`. ¿Por qué no un solo programa que haga todo?

La respuesta corta: **el agente corre en cada máquina que quieres monitorear. El servidor corre una sola vez, en el centro.**

---

## El modelo mental

Imagina que tienes 5 servidores corriendo tu aplicación:

```
  web-01          web-02          worker-01
┌──────────┐    ┌──────────┐    ┌──────────┐
│ [agente] │    │ [agente] │    │ [agente] │
│          │    │          │    │          │
│ cpu: 42% │    │ cpu: 71% │    │ cpu: 18% │
│ mem: 6GB │    │ mem: 7GB │    │ mem: 2GB │
└────┬─────┘    └────┬─────┘    └────┬─────┘
     │               │               │
     └───────────────┴───────────────┘
                     │  HTTP POST /api/v1/metrics
                     ▼
              ┌─────────────┐
              │  [servidor] │──── TimescaleDB
              │             │
              │  dashboard  │◄─── tu navegador
              │  alertas    │──── webhook Slack
              │  query API  │
              └─────────────┘
```

**Un servidor. Muchos agentes.** Este es el mismo modelo que usa Datadog — ellos lo llaman el Datadog Agent y el backend de Datadog. La diferencia es que con MiniObserv, tú eres dueño del servidor y lo corres tú mismo.

---

## Qué hace el Agente

El agente es un **recopilador y emisor**. No tiene base de datos, ni servidor web, ni dashboard. Su único trabajo es:

1. **Recopilar** — cada 10 segundos, lee uso de CPU, memoria, disco y red del sistema operativo
2. **Seguir logs** — observa archivos de log para detectar nuevas líneas (usando `fsnotify`)
3. **Enviar** — agrupa todo y hace POST al servidor con un JWT

```
Ciclo del agente (cada 10s):
  ┌────────────────────────────────────────────┐
  │  leer CPU de /proc/stat                    │
  │  leer memoria de /proc/meminfo             │
  │  leer disco via syscall                    │
  │  leer deltas de red de /proc/net/dev       │
  │  leer nuevas líneas de archivos de log     │
  │                                            │
  │  → agrupar todo en un batch                │
  │  → POST /api/v1/metrics  (JWT)             │
  │  → POST /api/v1/logs     (JWT)             │
  └────────────────────────────────────────────┘
```

El agente es **sin estado entre recopilaciones**. Si se cae y reinicia, simplemente empieza a recopilar de nuevo. No se almacena nada localmente.

El binario del agente es pequeño (~10 MB). Puedes colocarlo en cualquier servidor Linux, configurar dos variables de entorno y empieza a funcionar.

```bash
SERVER_URL=http://tu-servidor:8080 \
AGENT_TOKEN=tu-secreto \
./agent
```

---

## Qué hace el Servidor

El servidor es el **cerebro**. Recibe datos de todos los agentes, los almacena y los sirve de vuelta. Tiene cinco responsabilidades:

| Responsabilidad | Cómo |
|---|---|
| **Recibir métricas y logs** | Endpoints POST, valida JWT, inserta en bulk con `pgx.Batch` |
| **Almacenar** | Hypertable TimescaleDB (métricas), tabla normal (logs) |
| **Consultar** | API de time-bucket, búsqueda paginada de logs por cursor |
| **Alertar** | Ticker cada 30s evalúa reglas de umbral contra datos recientes |
| **Servir el dashboard** | HTML/JS embebido con `//go:embed`, sin paso de build |

El servidor necesita una base de datos (TimescaleDB). El agente no. Por eso son separados: no quieres instalar una base de datos en cada máquina que estás monitoreando.

---

## ¿Por qué no un solo binario?

Podrías pensar: "¿por qué no combinarlos?" Esto es lo que se rompería:

**1. Tendrías que instalar una base de datos en cada servidor**

Cada máquina que corra el binario combinado necesitaría TimescaleDB. Un agente de monitoreo debe ser ligero y sin dependencias externas. El agente hoy no tiene ninguna: sin base de datos, sin escritura en disco, sin puertos abiertos.

**2. Perderías la agregación N-a-1**

Con binarios separados, 50 agentes envían a 1 servidor. Con un binario combinado, cada máquina solo vería sus propios datos. El objetivo de un sistema de monitoreo es ver todas tus máquinas en un solo lugar.

**3. La seguridad sería más débil**

El agente solo envía datos — nunca los lee de vuelta. Si un agente es comprometido, el atacante puede inyectar métricas falsas pero no puede leer tu historial de monitoreo. Si los combinaras, cada agente tendría acceso de lectura a todo.

**4. La topología de red no funcionaría**

Los agentes envían datos hacia afuera (puerto 8080 en el servidor). No necesitan ningún puerto de entrada abierto. Esto funciona incluso cuando los agentes están detrás de firewalls o NAT. Un binario combinado necesitaría que cada máquina aceptara conexiones entrantes.

---

## El handshake JWT

Los agentes no tienen usuarios ni contraseñas. Usan un **secreto compartido de firma HS256** (`AGENT_TOKEN`). Cuando el agente inicia, usa este secreto para generar un JWT firmado válido por 24 horas. El servidor verifica la firma en cada request.

```
Agente                          Servidor
  │                               │
  │  AGENT_TOKEN = "mi-secreto"   │  AGENT_TOKEN = "mi-secreto"
  │                               │
  │  genera JWT (HS256, 24h TTL)  │
  │                               │
  │── POST /api/v1/metrics ──────►│
  │   Authorization: Bearer <JWT> │  verifica firma
  │                               │  ✓ mismo secreto → acepta
  │◄─ 202 Accepted ───────────────│
```

Tanto el agente como el servidor se configuran con el mismo `AGENT_TOKEN`. El agente lo usa para firmar tokens; el servidor para verificarlos. Ninguno almacena contraseñas ni certificados.

---

## En la práctica: correr ambos

**Desarrollo (misma máquina):**
```bash
# Terminal 1 — servidor
DATABASE_URL=postgres://... AGENT_TOKEN=dev-secret ./server

# Terminal 2 — agente (apuntando al servidor local)
SERVER_URL=http://localhost:8080 AGENT_TOKEN=dev-secret ./agent
```

**Producción (máquinas separadas):**
```bash
# En tu servidor de monitoreo
DATABASE_URL=postgres://... AGENT_TOKEN=secreto-produccion ./server

# En cada servidor que quieres monitorear
SERVER_URL=http://servidor-monitoreo:8080 AGENT_TOKEN=secreto-produccion ./agent
```

**Docker Compose (ambos en el mismo stack):**
```yaml
services:
  server:
    image: kamerrezz/miniobserv-server:latest
    environment:
      DATABASE_URL: postgres://minidog:minidog@db:5432/miniobserv
      AGENT_TOKEN: tu-secreto
    ports: ["8080:8080"]

  agent:
    image: kamerrezz/miniobserv-agent:latest
    environment:
      SERVER_URL: http://server:8080   # ← nombre del servicio en la red Docker
      AGENT_TOKEN: tu-secreto
```

---

## Resumen

| | Agente | Servidor |
|---|---|---|
| **Corre en** | Cada máquina monitoreada | Una máquina central |
| **Cantidad** | N (uno por host) | 1 |
| **Tiene base de datos** | No | Sí (TimescaleDB) |
| **Abre puertos** | No | Sí (8080) |
| **Tamaño del binario** | ~10 MB | ~15 MB |
| **Estado** | Sin estado | Con estado |
| **Reiniciable** | Instantáneamente | Necesita conexión a DB |
| **Imagen Docker** | `kamerrezz/miniobserv-agent` | `kamerrezz/miniobserv-server` |

La separación no es un capricho de diseño — es el único diseño que escala para monitorear múltiples máquinas. Todas las plataformas de observabilidad reales (Datadog, Prometheus, Grafana Cloud, Elastic) usan el mismo patrón: un recopilador ligero por host y un backend centralizado que agrega todo.
