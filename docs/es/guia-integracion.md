# Guía de integración de MiniObserv para proyectos Node.js

Esta guía está dirigida a desarrolladores que trabajan con Express.js, NestJS o Next.js y quieren tener visibilidad real sobre lo que su aplicación hace en producción — latencia de peticiones, tasa de errores, presión de memoria, uso de disco — sin incorporar una plataforma de observabilidad pesada.

---

## 1. Qué cubre realmente MiniObserv

Antes de integrarlo, es importante saber qué capa de observabilidad cubre MiniObserv — y cuál no.

```
Capa 1 — Infraestructura        ← MiniObserv
  ¿La máquina está bien?
  CPU, memoria, disco, red, líneas de log
  Herramientas: MiniObserv, Datadog, Prometheus

Capa 2 — Aplicación (APM)
  ¿Mi código está bien?
  Queries lentos, N+1, stack traces, latencia por función
  Herramientas: Sentry, Datadog APM, OpenTelemetry

Capa 3 — Negocio
  ¿Mi producto está bien?
  Usuarios activos, conversiones, uso de features
  Herramientas: PostHog, Mixpanel, Amplitude
```

**MiniObserv es Capa 1.** Monitorea el servidor donde corre tu app — el sistema operativo, no tu código ni tus datos. No se conecta a tu PostgreSQL, no lee tu esquema de Prisma, no sabe nada de tu lógica de negocio. Cuando despliegas en 3 servidores, te dice que el CPU de cada uno está al 42% — no te dirá qué query de Prisma causó un pico de latencia (eso es Capa 2).

En producción eventualmente necesitarás las tres capas. Empieza con la Capa 1 — si el servidor se queda sin memoria o disco, las otras dos no importan.

---

## 2. Por qué MiniObserv para proyectos Node.js

Tu framework funciona bien. Lo que no tienes es visibilidad sobre lo que ocurre una vez desplegado.

**Lo que obtienes sin escribir código:**

- CPU, memoria, disco y red recolectados cada 10 segundos
- Todas las líneas de log de tu aplicación en un visor estructurado
- Alertas cuando los recursos del sistema superan los umbrales que definas
- Dashboard con gráficas en tiempo real en `http://localhost:8080`

El agente de MiniObserv es un binario Go independiente. Corre **junto a** tu aplicación Node.js como un contenedor separado. No conoce Node.js — monitorea el sistema operativo del host. No necesitas instalar nada en tu app para obtener métricas a nivel de sistema.

**El SDK (`@kamerrezz/miniobserv`) es opcional.** Úsalo solo cuando necesites enviar métricas personalizadas a nivel de aplicación (distribución de tiempos de respuesta, conteo de errores, profundidad de colas). Empieza sin él — es posible que no lo necesites.

---

## 2. Configuración inicial (3 minutos)

Agrega MiniObserv a tu `docker-compose.yml` existente. Necesitas **tres nuevos servicios** junto a los tuyos:

| Servicio | Qué es | ¿Toca tu app? |
|---|---|---|
| `miniobserv-db` | TimescaleDB — almacenamiento **propio** de MiniObserv | No |
| `miniobserv` | El servidor MiniObserv + dashboard | No |
| `miniobserv-agent` | Recolecta métricas del sistema operativo | No |

> **`miniobserv-db` es completamente independiente de tu base de datos.** Si tu stack usa PostgreSQL + Prisma, MySQL + Sequelize, o cualquier otra cosa — MiniObserv nunca la toca. Tiene su propio contenedor de base de datos aislado. Tu app y MiniObserv no comparten ningún almacenamiento.

### Stack típico: Express + Prisma + PostgreSQL

Así se ve un proyecto real con MiniObserv integrado:

```yaml
services:
  # ── tu stack existente ───────────────────────────────────────
  db:
    image: postgres:16
    environment:
      POSTGRES_USER: miapp
      POSTGRES_PASSWORD: miapp
      POSTGRES_DB: miapp_db
    volumes:
      - app_data:/var/lib/postgresql/data

  app:
    build: .
    environment:
      DATABASE_URL: "postgresql://miapp:miapp@db:5432/miapp_db"
    depends_on:
      - db
      - miniobserv        # opcional: esperar a que el dashboard esté listo
    volumes:
      - app_logs:/var/log/app

  # ── MiniObserv — agrega estos tres servicios ─────────────────
  miniobserv-db:          # DB propia de MiniObserv — aislada de la tuya
    image: timescale/timescaledb:latest-pg16
    environment:
      POSTGRES_USER: minidog
      POSTGRES_PASSWORD: minidog
      POSTGRES_DB: miniobserv
    volumes:
      - miniobserv_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U minidog -d miniobserv"]
      interval: 5s
      retries: 10

  miniobserv:
    image: kamerrezz/miniobserv-server:latest
    environment:
      DATABASE_URL: "postgres://minidog:minidog@miniobserv-db:5432/miniobserv?sslmode=disable"
      AGENT_TOKEN: "tu-secreto-de-32-caracteres-aqui"
      ALERT_RULES: '[{"host":"*","name":"cpu.usage_pct","op":">","threshold":80,"for":"5m"}]'
    ports:
      - "8080:8080"
    depends_on:
      miniobserv-db:
        condition: service_healthy

  miniobserv-agent:
    image: kamerrezz/miniobserv-agent:latest
    environment:
      SERVER_URL: "http://miniobserv:8080"
      AGENT_TOKEN: "tu-secreto-de-32-caracteres-aqui"
      LOG_PATHS: "/var/log/app/app.log"
    volumes:
      - app_logs:/var/log/app:ro   # acceso de solo lectura a los logs de tu app

volumes:
  app_data:           # datos de tu app
  miniobserv_data:    # datos de MiniObserv — completamente separados
  app_logs:           # compartido entre app (escritura) y agente (lectura)
```

**Genera un `AGENT_TOKEN` seguro:**

```bash
openssl rand -hex 32
```

Usa el mismo valor en `miniobserv` y en `miniobserv-agent`. Nunca sale de tu infraestructura.

Abre `http://localhost:8080` después de `docker compose up`. El dashboard aparece en aproximadamente 10 segundos.

---

## 3. Integración con Express.js

### 3a. Escribir logs en un archivo (para el seguimiento de logs)

El agente de MiniObserv observa los archivos indicados en `LOG_PATHS` y transmite cada línea nueva al visor de logs del dashboard. La forma más sencilla es añadir un transport de archivo al logger de Winston existente:

```javascript
const winston = require('winston');

const logger = winston.createLogger({
  transports: [
    new winston.transports.Console(),
    new winston.transports.File({ filename: '/var/log/myapp/app.log' }),
  ],
  format: winston.format.combine(
    winston.format.timestamp(),
    winston.format.json(),
  ),
});
```

Monta el mismo volumen en el contenedor de tu app para que el agente pueda leer el archivo:

```yaml
services:
  api:
    volumes:
      - app_logs:/var/log/myapp
```

### 3b. Middleware de registro de peticiones (formato estructurado)

Registra cada petición para que el flujo de logs del dashboard muestre actividad HTTP significativa. El nivel se determina por el código de estado — ERROR para 5xx, WARN para 4xx, INFO para el resto:

```javascript
app.use((req, res, next) => {
  res.on('finish', () => {
    const level =
      res.statusCode >= 500 ? 'error' :
      res.statusCode >= 400 ? 'warn' : 'info';

    logger[level]({
      method: req.method,
      path: req.path,
      status: res.statusCode,
    });
  });
  next();
});
```

Esto es suficiente para ver errores HTTP y rutas lentas en el dashboard sin usar el SDK.

### 3c. Enviar métricas personalizadas con el SDK (opcional)

Instala el SDK:

```bash
npm install @kamerrezz/miniobserv
```

Inicializa el cliente una vez y reutilízalo en toda la aplicación:

```javascript
import { MiniObservClient } from '@kamerrezz/miniobserv';

const obs = new MiniObservClient({
  baseUrl: process.env.MINIOBSERV_URL ?? 'http://miniobserv:8080',
  agentToken: process.env.AGENT_TOKEN,
  defaultHost: process.env.HOSTNAME ?? 'api-server',
});
```

Envía una señal personalizada desde un middleware — disparar y olvidar, nunca bloquear la petición:

```javascript
app.use((req, res, next) => {
  const start = Date.now();
  res.on('finish', () => {
    const heapUsed = process.memoryUsage().heapUsed;
    const heapTotal = process.memoryUsage().heapTotal;

    obs.pushMetric('mem.used_pct', (heapUsed / heapTotal) * 100)
      .catch(() => {}); // nunca dejes que la observabilidad rompa la petición
  });
  next();
});
```

**Una nota sobre los nombres de métricas:** MiniObserv tiene un conjunto fijo de nombres canónicos (`cpu.usage_pct`, `mem.used_pct`, `disk.used_pct`, etc.). El agente ya los completa desde el sistema operativo. Al enviar desde el SDK, puedes usar los mismos nombres para añadir puntos de datos desde la perspectiva de la app, o enviar cualquier valor que quieras rastrear. El enfoque recomendado para la mayoría de apps es comenzar con el patrón sidecar (solo el agente) y añadir envíos por SDK únicamente para señales de alto valor como tasa de errores o latencia de base de datos.

---

## 4. Integración con NestJS

### 4a. Interceptor global para el registro de peticiones

Crea un interceptor que registre cada petición con método, URL, estado y duración:

```typescript
// src/common/observability.interceptor.ts
import {
  Injectable,
  NestInterceptor,
  ExecutionContext,
  CallHandler,
  Logger,
} from '@nestjs/common';
import { Observable, tap } from 'rxjs';

@Injectable()
export class ObservabilityInterceptor implements NestInterceptor {
  private readonly logger = new Logger('HTTP');

  intercept(context: ExecutionContext, next: CallHandler): Observable<any> {
    const req = context.switchToHttp().getRequest();
    const start = Date.now();

    return next.handle().pipe(
      tap(() => {
        const res = context.switchToHttp().getResponse();
        const duration = Date.now() - start;
        this.logger.log(
          `${req.method} ${req.url} → ${res.statusCode} (${duration}ms)`,
        );
      }),
    );
  }
}
```

### 4b. Registro global en `main.ts`

```typescript
// main.ts
import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';
import { ObservabilityInterceptor } from './common/observability.interceptor';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);
  app.useGlobalInterceptors(new ObservabilityInterceptor());
  await app.listen(3000);
}
bootstrap();
```

### 4c. Redirigir los logs de NestJS a un archivo con Winston

```bash
npm install nest-winston winston
```

```typescript
// src/winston.config.ts
import * as winston from 'winston';

export const winstonConfig: winston.LoggerOptions = {
  transports: [
    new winston.transports.Console(),
    new winston.transports.File({ filename: '/var/log/myapp/app.log' }),
  ],
  format: winston.format.combine(
    winston.format.timestamp(),
    winston.format.json(),
  ),
};
```

```typescript
// main.ts
import { WinstonModule } from 'nest-winston';
import { winstonConfig } from './winston.config';

const app = await NestFactory.create(AppModule, {
  logger: WinstonModule.createLogger(winstonConfig),
});
```

Con esto en su lugar, cada log de NestJS — incluyendo la salida del interceptor — aterriza en `/var/log/myapp/app.log`, que el agente de MiniObserv observa automáticamente.

---

## 5. Integración con Next.js

### 5a. Middleware de rutas API (App Router, Next.js 14+)

El middleware de Next.js corre en el Edge y no tiene acceso a las APIs de archivo de Node.js. La forma más sencilla de llevar logs a MiniObserv es escribir JSON estructurado a stdout y redirigir la salida de Docker a un archivo.

```typescript
// middleware.ts
import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

export function middleware(request: NextRequest) {
  const start = Date.now();
  const response = NextResponse.next();

  console.log(
    JSON.stringify({
      level: 'INFO',
      method: request.method,
      path: request.nextUrl.pathname,
      duration_ms: Date.now() - start,
    }),
  );

  return response;
}

export const config = {
  matcher: '/api/:path*',
};
```

### 5b. Redirigir el stdout de Next.js a un archivo (docker-compose)

Redirige la salida del contenedor a un archivo que el agente pueda seguir:

```yaml
services:
  nextjs:
    command: sh -c "node server.js 2>&1 | tee /var/log/myapp/app.log"
    volumes:
      - app_logs:/var/log/myapp
```

El comando `tee` escribe tanto en stdout (para que `docker logs` siga funcionando) como en el archivo. El agente de MiniObserv recoge las nuevas líneas automáticamente a través de `LOG_PATHS`.

---

## 6. Reglas de alerta para aplicaciones Node.js

Define las reglas de alerta como un arreglo JSON en la variable de entorno `ALERT_RULES` del servidor de MiniObserv. Cada regla se activa cuando la condición se mantiene durante la duración especificada:

```json
[
  { "host": "*", "name": "cpu.usage_pct",  "op": ">", "threshold": 80, "for": "5m"  },
  { "host": "*", "name": "mem.used_pct",   "op": ">", "threshold": 85, "for": "10m" },
  { "host": "*", "name": "disk.used_pct",  "op": ">", "threshold": 90, "for": "1m"  }
]
```

- `host: "*"` coincide con todos los hosts que reportan a este servidor
- `for` es la duración mínima sostenida antes de que se dispare la alerta — evita ruido por picos transitorios
- Las alertas disparadas aparecen en el dashboard; configura `ALERT_NOTIFICATIONS` para recibir también notificaciones por webhook (ver [Sección 8](#8-notificaciones-y-salud-de-hosts))

Un punto de partida práctico para una API Node.js típica:

| Métrica | Umbral | Duración | Motivo |
|---|---|---|---|
| `cpu.usage_pct` | 80% | 5m | Carga sostenida, no un pico aislado |
| `mem.used_pct` | 85% | 10m | Las fugas de memoria crecen lentamente |
| `disk.used_pct` | 90% | 1m | El disco lleno es inmediato y fatal |

---

## 8. Notificaciones y salud de hosts

### 8a. Notificaciones por webhook

MiniObserv puede notificar a servicios externos cuando una alerta se activa o se resuelve. Establece `ALERT_NOTIFICATIONS` en el servidor con un arreglo JSON de destinos webhook:

```yaml
miniobserv:
  environment:
    ALERT_NOTIFICATIONS: '[{"type":"webhook","url":"https://hooks.slack.com/services/YOUR/WEBHOOK/URL"}]'
```

Funciona con Slack, Discord, Teams, PagerDuty o cualquier servicio que acepte un POST con JSON. Cada notificación se entrega con un timeout de 5 segundos (dispara y olvida, sin reintentos en v1).

**Formato del payload:**

```json
{"event":"firing","rule":{"host":"*","name":"mem.used_pct","op":">","threshold":85,"for":"10m"},"value":87.4,"fired_at":"2026-06-05T16:42:23Z"}
```

`event` vale `"firing"` cuando se supera el umbral y `"resolved"` cuando la métrica vuelve a estar por debajo.

### 8b. Alertas automáticas de host caído

El servidor registra el último heartbeat de cada agente conectado. Si un agente deja de reportar métricas — por un crash del contenedor, kill por OOM, partición de red u otro motivo — MiniObserv enviará automáticamente un webhook `host.down` una vez que el host lleve silencioso más tiempo del indicado en `HOST_DOWN_AFTER` (predeterminado: 50s).

No se necesita configuración adicional más allá de establecer `ALERT_NOTIFICATIONS`. No hace falta escribir una regla de alerta para esto — el estado de los hosts se rastrea de forma automática.

| Variable | Predeterminado | Significado |
|----------|----------------|-------------|
| `HOST_STALE_AFTER` | `20s` | El host se vuelve naranja (stale) en el dashboard |
| `HOST_DOWN_AFTER` | `50s` | El host se vuelve rojo (down) y se dispara el webhook |

Puedes consultar el estado actual de todos los hosts con `GET /api/v1/hosts` (público, sin autenticación):

```bash
curl -s http://localhost:8080/api/v1/hosts | jq .
```

### 8c. Snippet actualizado de docker-compose con notificaciones

```yaml
miniobserv:
  build:
    context: ./path/to/theminidog
    dockerfile: Dockerfile.server
  environment:
    DATABASE_URL: "postgres://minidog:minidog@miniobserv-db:5432/miniobserv?sslmode=disable"
    AGENT_TOKEN: "tu-secreto-de-32-caracteres-aqui"
    ALERT_RULES: '[{"host":"*","name":"cpu.usage_pct","op":">","threshold":80,"for":"5m"},{"host":"*","name":"mem.used_pct","op":">","threshold":85,"for":"10m"}]'
    ALERT_NOTIFICATIONS: '[{"type":"webhook","url":"https://hooks.slack.com/services/YOUR/WEBHOOK/URL"}]'
    HOST_STALE_AFTER: "20s"
    HOST_DOWN_AFTER: "50s"
  ports:
    - "8080:8080"
  depends_on:
    miniobserv-db:
      condition: service_healthy
```

---

## 7. Lo que obtienes sin escribir una sola línea de SDK

Si solo agregas los dos servicios de MiniObserv a tu docker-compose y apuntas `LOG_PATHS` al archivo de log de tu app, obtienes:

- **Métricas de sistema** — CPU, memoria, disco y red recolectados cada 10 segundos, de forma automática
- **Flujo de logs** — cada línea que tu app escribe en el archivo de log, visible en el dashboard con marcas de tiempo
- **Alertas** — notificaciones cuando los recursos superan los umbrales que definiste
- **Dashboard en vivo** — gráficas en tiempo real en `http://localhost:8080`, sin configuración adicional

El SDK es para cuando necesitas ir más lejos: rastrear tasas de error por ruta, medir la latencia de consultas a la base de datos, registrar la profundidad de colas. Empieza sin él. Incorpóralo cuando tengas una señal específica que no puedas observar a partir de los logs y las métricas de sistema.

---

## Referencia

### Nombres canónicos de métricas

| Nombre | Unidad | Descripción |
|---|---|---|
| `cpu.usage_pct` | % | Utilización de CPU en todos los núcleos |
| `mem.used_pct` | % | Memoria usada como porcentaje del total |
| `mem.used_bytes` | bytes | Memoria actualmente en uso |
| `mem.total_bytes` | bytes | Memoria total instalada |
| `disk.used_pct` | % | Porcentaje de disco usado (mount `/`) |
| `disk.used_bytes` | bytes | Espacio en disco en uso |
| `disk.total_bytes` | bytes | Capacidad total del disco |
| `net.bytes_in` | bytes | Bytes de red recibidos (delta por intervalo) |
| `net.bytes_out` | bytes | Bytes de red enviados (delta por intervalo) |

> Las métricas `net.*` son valores delta — no se emiten datos en el primer intervalo de recolección.

### Variables de entorno del agente

| Variable | Obligatoria | Predeterminado | Descripción |
|---|---|---|---|
| `SERVER_URL` | sí | — | URL base del servidor de MiniObserv |
| `AGENT_TOKEN` | sí | — | Secreto HS256 compartido (mínimo 16 caracteres) |
| `COLLECT_INTERVAL` | no | `10s` | Frecuencia de recolección (1s–300s) |
| `LOG_PATHS` | no | — | Rutas separadas por coma para el tail de logs |
| `AGENT_HOST` | no | hostname del SO | Etiqueta de host en todas las métricas |
