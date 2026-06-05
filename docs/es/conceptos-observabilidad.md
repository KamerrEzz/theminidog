# Conceptos de Observabilidad: una guía para empezar desde cero

Si llegaste acá sin saber qué es una métrica, un log, o por qué existe Datadog, este documento es para vos. La idea es que, al terminar de leerlo, entiendas no solo los conceptos sino también cómo aparecen en MiniObserv — el proyecto concreto que estás usando para aprenderlos.

---

## 🔍 ¿Qué es la Observabilidad?

Imaginá que desplegás una aplicación. Funciona en tu máquina. La subís a producción. Y ahora... ¿cómo sabés que sigue funcionando? ¿Cómo sabés que está respondiendo lento? ¿Cómo sabés que se cayó a las 3am mientras dormías?

Sin ningún sistema de monitoreo, estás **volando a ciegas**. Solo sabés que algo está mal cuando un usuario se queja — o peor, cuando el negocio ya perdió plata.

Un primer intento de solución es agregar **logs**: mensajes de texto que la aplicación escribe cuando sucede algo. Eso ayuda, pero es como mirar por el espejo retrovisor. Sabés qué pasó, pero después de que pasó. Y si tenés muchos logs, encontrar el problema relevante es como buscar una aguja en un pajar.

La observabilidad es el **dashboard completo del auto**: velocímetro (¿qué tan rápido va el sistema?), luz de check engine (¿algo está fuera de rango?), cuenta kilómetros (¿cuántas solicitudes procesó?), y la bitácora del mecánico (¿qué eventos ocurrieron y por qué?). Combinando todo eso, podés entender el estado de tu sistema en tiempo real y diagnosticar problemas con precisión.

En MiniObserv, esto se ve así: el agente corre en tu máquina, recolecta CPU, RAM, disco y red cada 10 segundos, y los envía al servidor. Vos podés preguntar después: "¿Cómo estuvo el CPU de `web-01` en la última hora?" y obtener una respuesta con datos reales.

---

## 🏛️ Los Tres Pilares

La industria organiza la observabilidad en tres categorías principales.

**Métricas** son números en el tiempo. "La CPU estuvo al 90% entre las 14:00 y las 15:00." "La RAM usada llegó a 7.2 GB a las 14:32." Son agregables, comparables y perfectas para detectar tendencias y disparar alertas.

**Logs** son eventos que sucedieron. "El usuario 123 inició sesión a las 14:32:01." "La consulta a la base de datos falló con error de timeout." Son texto libre, ricos en detalle y útiles para entender el *por qué* de un problema.

**Trazas** son el recorrido de una solicitud individual a través de múltiples servicios. Cuando un usuario hace clic en "comprar", esa acción puede pasar por un servicio de autenticación, luego uno de inventario, luego uno de pagos. Una traza registra ese recorrido completo, con el tiempo que tomó cada paso.

MiniObserv cubre **métricas y logs**. Las trazas no están implementadas porque requieren instrumentar cada servicio individualmente y correlacionarlos con un ID único — son la herramienta más compleja de los tres pilares y tienen más sentido en arquitecturas de microservicios. Para empezar a entender observabilidad, métricas y logs son más que suficientes.

---

## 📊 ¿Qué es una Métrica?

Una métrica tiene cuatro partes:

- **Nombre**: qué estás midiendo (`cpu.usage_pct`)
- **Valor**: el número medido (`42.5`)
- **Timestamp**: cuándo se midió (`2026-06-05T10:00:00Z`)
- **Etiquetas (labels)**: contexto adicional (`host="web-01"`, `core="total"`)

En MiniObserv, una métrica real luce así:

```
nombre:    cpu.usage_pct
valor:     42.5
tiempo:    2026-06-05T10:00:00Z
host:      web-01
etiquetas: {"core": "total"}
```

Las métricas que recolecta el agente son:

| Métrica | Qué mide |
|---|---|
| `cpu.usage_pct` | Porcentaje de uso de CPU (total y por núcleo) |
| `mem.used_pct` | Porcentaje de RAM usada |
| `mem.used_bytes` | RAM usada en bytes |
| `mem.total_bytes` | RAM total en bytes |
| `disk.used_pct` | Porcentaje de disco usado (por punto de montaje) |
| `disk.used_bytes` | Disco usado en bytes |
| `disk.total_bytes` | Disco total en bytes |
| `net.bytes_in` | Bytes recibidos por interfaz de red |
| `net.bytes_out` | Bytes enviados por interfaz de red |

Hay tres tipos comunes de métricas. Un **gauge** es un valor que puede subir o bajar libremente — como la temperatura o el porcentaje de CPU. Un **counter** solo puede crecer: el total de solicitudes HTTP procesadas desde que arrancó el servidor. Un **histograma** agrupa observaciones en rangos para medir distribuciones — como cuántas solicitudes tardaron entre 0-10ms, entre 10-50ms, etc. MiniObserv trabaja principalmente con gauges y counters delta (la red mide bytes desde el último tick, no acumulados).

### Por qué las series de tiempo se guardan diferente

Imaginate que el agente envía una muestra de CPU cada 10 segundos. En una hora, eso es 360 filas solo para una máquina. En un mes, más de 260.000 filas. Si tenés 10 servidores y 9 métricas cada uno, multiplicá.

Una base de datos relacional normal escanea toda la tabla para encontrar "la última hora". TimescaleDB divide esa tabla en **chunks** — rebanadas de tiempo, un día por chunk según la migración de MiniObserv. Para consultar "la última hora", solo lee el chunk del día de hoy. Todo lo anterior ni lo toca.

El modelo mental es simple:

```
Cada 10 segundos, el agente registra:
  cpu.usage_pct = 42.5
  cpu.usage_pct = 43.1
  cpu.usage_pct = 41.8
  ... (360 puntos en una hora)

Vos preguntás: "CPU promedio cada 5 minutos en la última hora"
  → 12 puntos de datos, perfecto para graficar
```

La función que hace eso se llama `time_bucket`. En la query real de MiniObserv:

```sql
SELECT time_bucket('5 minutes', time) AS bucket,
       avg(value) AS value
FROM metrics
WHERE host = 'web-01'
  AND name = 'cpu.usage_pct'
  AND time >= $3
  AND time <= $4
GROUP BY bucket
ORDER BY bucket DESC
```

La tabla `metrics` es una **hypertable**: una tabla normal en apariencia, pero que TimescaleDB gestiona internamente dividiéndola en chunks por tiempo. No tenés que hacer nada especial — simplemente la creás con `create_hypertable()` y TimescaleDB se encarga del resto.

---

## 📋 ¿Qué es un Log?

Un log es un mensaje de texto que tu aplicación escribe cuando algo sucede. Tiene cuatro elementos clave:

- **Tiempo**: cuándo ocurrió
- **Nivel de severidad**: qué tan importante es
- **Mensaje**: qué pasó
- **Contexto**: información adicional (user_id, host, etc.)

```
2026-06-05T14:32:01Z  INFO   login exitoso            user_id=123
2026-06-05T14:32:15Z  ERROR  fallo en base de datos   err="connection timeout"
2026-06-05T14:33:00Z  WARN   batch channel full       host=web-01
```

Los niveles de severidad, de menor a mayor importancia:

- **DEBUG**: muy detallado, solo útil mientras desarrollás. En producción genera demasiado ruido.
- **INFO**: eventos normales del sistema. El agente arrancó, procesó un batch, se conectó al servidor.
- **WARN**: algo raro pero no roto. En MiniObserv, el agente loguea WARN cuando un collector falla o cuando el canal de batches está lleno.
- **ERROR**: algo se rompió y requiere atención. Un fallo de red persistente, un error de base de datos.

¿Por qué importan los logs? Las métricas te dicen **QUÉ** está mal: "el CPU de web-01 llegó al 95% a las 14:30". Los logs te dicen **POR QUÉ**: "a las 14:30, la consulta de base de datos tardó 45 segundos porque se agotó el pool de conexiones". Los dos son complementarios — sin ambos, solo tenés la mitad del diagnóstico.

---

## 🗺️ Cómo Funciona MiniObserv (el panorama completo)

El sistema tiene dos componentes principales que se comunican por HTTP.

```
Tu computadora / servidor
        │
   ┌────┴──────┐
   │  AGENTE   │
   │           │  Lee CPU, RAM, Disco, Red cada 10s
   │           │  (via gopsutil — habla con el SO)
   └────┬──────┘
        │
        │  POST /api/v1/metrics
        │  Authorization: Bearer <JWT>
        │  { "host": "web-01", "metrics": [...] }
        │
        ▼
   ┌────────────┐
   │  SERVIDOR  │
   │            │  Valida el JWT
   │            │  Valida el batch
   │            │  Almacena en TimescaleDB
   └────┬───────┘
        │
        │  GET /api/v1/metrics/query
        │  ?host=web-01&name=cpu.usage_pct
        │  &from=...&to=...&bucket=5m&agg=avg
        │
        ▼
   ┌───────────────┐
   │  VOS / TU     │
   │  EQUIPO       │
   │               │  Recibís los puntos agrupados
   │               │  para graficar o analizar
   └───────────────┘
```

**El agente** es un proceso liviano escrito en Go que corre en la máquina que querés monitorear. Usa `gopsutil` — una librería que le pregunta directamente al sistema operativo por estadísticas de CPU, RAM, disco y red. Cada 10 segundos recolecta todas las métricas y las empaqueta en un `MetricBatch`, que envía al servidor vía HTTP con reintento automático y backoff exponencial (si falla, espera 1s, luego 2s, luego 4s, hasta un máximo de 60s).

**El servidor** recibe los batches, valida que el JWT sea correcto y que las métricas sean conocidas, y las guarda en TimescaleDB. También expone el endpoint de consulta para que puedas recuperar los datos históricos. Los endpoints públicos `/healthz` y `/readyz` no requieren autenticación — son para que infraestructura pueda verificar que el servidor está vivo.

---

## 🔐 ¿Por Qué JWT para la Autenticación?

El agente necesita probarle al servidor que está autorizado a enviar datos. Sin autenticación, cualquier proceso en la red podría inundar tu servidor con métricas falsas — o directamente con basura que rompa el almacenamiento.

La solución de MiniObserv usa JWT (JSON Web Token). Un JWT es un mensaje con tres partes: un header (qué algoritmo se usa), un payload (los datos, como quién eres y cuándo expira) y una firma. La firma se calcula con HMAC-SHA256 usando un secreto que solo conocen el agente y el servidor.

Cuando el agente arranca, genera un JWT firmado con ese secreto compartido y lo incluye en cada request con el header `Authorization: Bearer <token>`. El servidor recibe el token, recalcula la firma con su copia del secreto, y las compara. Si coinciden, el mensaje es auténtico. Si no coinciden, alguien modificó el token o no conoce el secreto.

La analogía clásica es el **sello de lacre** en una carta: cualquiera puede leer la carta, pero solo quien tiene el sello original puede crear uno nuevo que parezca auténtico. En MiniObserv, el secreto vive en la variable de entorno `AGENT_TOKEN`. El agente lo usa para firmar un JWT que expira en 24 horas.

MiniObserv usa específicamente `HS256` (HMAC-SHA256) y rechaza explícitamente el algoritmo `none` — un ataque conocido donde un token sin firma se presenta como válido aprovechando implementaciones descuidadas de JWT.

---

## 🤖 ¿Qué es un Agente?

El patrón de agente es muy común en observabilidad: un proceso liviano que corre junto a tu aplicación y "reporta" datos a un sistema centralizado.

La clave es que el agente no interfiere con tu aplicación real. Corre de forma independiente, consume muy pocos recursos, y su único trabajo es recolectar y enviar datos.

Hay dos modelos de transporte: **push** y **pull**. En el modelo push, el agente toma la iniciativa y envía datos al servidor periódicamente — es lo que hace MiniObserv. En el modelo pull, el servidor pide datos al agente cuando los necesita — es lo que hace Prometheus, por ejemplo. Push encaja bien en MiniObserv porque el agente es el que tiene acceso directo al SO de esa máquina; el servidor no necesita saber cómo llegar a cada agente.

El agente de MiniObserv tiene dos loops corriendo en paralelo: el **collector loop** que recolecta métricas cada 10 segundos y las pone en un buffer, y el **sender loop** que consume ese buffer y las envía al servidor. Si la red está caída, el sender loop espera con backoff; el collector loop sigue corriendo para no perder datos recientes.

---

## 🔌 Entendiendo la API

Las dos operaciones principales de MiniObserv son ingesta y consulta.

**Ingestión** (el agente enviando datos):
```
POST /api/v1/metrics
Authorization: Bearer <JWT>
Content-Type: application/json

{
  "host": "web-01",
  "metrics": [
    {
      "time": "2026-06-05T14:32:00Z",
      "name": "cpu.usage_pct",
      "value": 42.5,
      "labels": {"core": "total"}
    }
  ]
}

→ 202 Accepted
{"ingested": 1}
```

**Consulta** (vos preguntando por datos históricos):
```
GET /api/v1/metrics/query
  ?host=web-01
  &name=cpu.usage_pct
  &from=2026-06-05T13:00:00Z
  &to=2026-06-05T14:00:00Z
  &bucket=5m
  &agg=avg
Authorization: Bearer <JWT>

→ 200 OK
{
  "host": "web-01",
  "name": "cpu.usage_pct",
  "bucket": "5m",
  "agg": "avg",
  "points": [
    {"time": "2026-06-05T13:55:00Z", "value": 41.2},
    {"time": "2026-06-05T13:50:00Z", "value": 43.8},
    ...
  ]
}
```

Los parámetros `bucket` y `agg` controlan cómo se agregan los datos. `bucket=5m` significa "agrupá los datos en ventanas de 5 minutos". `agg=avg` significa "calculá el promedio dentro de cada ventana". También podés usar `max` o `min`. Los buckets válidos son `1m`, `5m`, `15m`, `1h` y `1d`.

---

## 📖 Glosario

**Observabilidad**: la capacidad de entender el estado interno de un sistema a partir de sus salidas externas (métricas, logs, trazas). Un sistema observable te permite responder "¿qué está pasando y por qué?" sin tener que desplegar código nuevo para investigar.

**Métrica / Serie de tiempo**: un valor numérico medido periódicamente y asociado a un timestamp. La secuencia de esas mediciones en el tiempo forma una "serie de tiempo".

**Gauge**: tipo de métrica cuyo valor puede subir o bajar libremente. Ejemplo: `cpu.usage_pct`, `mem.used_pct`.

**Counter**: tipo de métrica que solo crece (o se reinicia al cero cuando el proceso arranca). Ejemplo: total de solicitudes HTTP. `net.bytes_in` y `net.bytes_out` en MiniObserv son contadores del SO que se convierten en deltas por intervalo.

**Log / Nivel de log**: un mensaje de texto con timestamp, severidad (DEBUG, INFO, WARN, ERROR) y contexto. Los niveles permiten filtrar el ruido y enfocarse en lo importante.

**Agente**: proceso liviano que corre en una máquina y recolecta o envía datos a un sistema centralizado. Diseñado para tener bajo impacto en los recursos del host.

**Hypertable / Chunk**: en TimescaleDB, una hypertable es una tabla de PostgreSQL que se divide automáticamente en chunks por tiempo. Cada chunk cubre un intervalo (en MiniObserv, 1 día). Las consultas por rango de tiempo solo leen los chunks relevantes, lo que las hace mucho más rápidas.

**JWT / HMAC**: JWT (JSON Web Token) es un estándar para crear tokens de autenticación firmados. HMAC-SHA256 es el algoritmo de firma que usa MiniObserv — calcula un hash del contenido del token usando un secreto compartido. Si el contenido cambia, el hash no coincide.

**Ingestión**: el proceso de recibir datos del exterior y almacenarlos. El endpoint `POST /api/v1/metrics` es el punto de ingestión de MiniObserv.

**Cardinalidad**: la cantidad de combinaciones únicas de etiquetas (labels) en tus métricas. Alta cardinalidad significa muchas combinaciones — por ejemplo, si usaras el `user_id` como etiqueta, tendrías tantas series de tiempo como usuarios. Eso puede explotar el almacenamiento. En MiniObserv la cardinalidad es baja y controlada: `core`, `mount` e `iface` tienen un número fijo de valores posibles.

**Pull vs Push**: en el modelo pull, el servidor pregunta al agente periódicamente. En el modelo push, el agente envía datos al servidor por iniciativa propia. MiniObserv usa push — el agente publica sus datos cada 10 segundos sin esperar que nadie los pida.
