# MiniObserv Integration Guide for Node.js Projects

This guide is for developers running Express.js, NestJS, or Next.js who want real visibility into what their app is doing in production — request latency, error rates, memory pressure, disk usage — without adding a heavy observability stack.

---

## 1. Why MiniObserv for Node.js projects

Your framework is solid. What you don't have is visibility into what happens once it's deployed.

**What you get out of the box — zero code required:**

- CPU, memory, disk, and network metrics collected every 10 seconds
- All your app's log lines surfaced in a structured dashboard log stream
- Alerts when system resources exceed thresholds you define
- Live sparkline dashboard at `http://localhost:8080`

The MiniObserv agent is a standalone Go binary. It runs **alongside** your Node.js app as a separate container or process. It knows nothing about Node.js — it monitors the host OS. You don't need to install anything in your app to get system-level metrics.

**The SDK (`@kamerrezz/miniobserv`) is optional.** Use it only when you need to push custom application-level metrics (response time distributions, error counts, queue depths). Start without it — you may not need it.

---

## 2. Quick Setup (3 minutes)

Add MiniObserv to your existing `docker-compose.yml`. You need two new services: the MiniObserv server (which stores metrics and serves the dashboard) and the agent (which collects and pushes metrics).

```yaml
# Add to your existing docker-compose.yml
services:
  # ... your existing services ...

  miniobserv-db:
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
    # Note: image not yet published — build from source
    build:
      context: ./path/to/theminidog
      dockerfile: Dockerfile.server
    environment:
      DATABASE_URL: "postgres://minidog:minidog@miniobserv-db:5432/miniobserv?sslmode=disable"
      AGENT_TOKEN: "your-32-char-secret-here"
      ALERT_RULES: '[{"host":"*","name":"cpu.usage_pct","op":">","threshold":80,"for":"5m"}]'
    ports:
      - "8080:8080"
    depends_on:
      miniobserv-db:
        condition: service_healthy

  miniobserv-agent:
    build:
      context: ./path/to/theminidog
      dockerfile: Dockerfile.agent
    environment:
      SERVER_URL: "http://miniobserv:8080"
      AGENT_TOKEN: "your-32-char-secret-here"
      COLLECT_INTERVAL: "10s"
      LOG_PATHS: "/var/log/myapp/app.log"  # path to your app's log file
    volumes:
      - app_logs:/var/log/myapp  # shared with your app container

volumes:
  miniobserv_data:
  app_logs:
```

**Generate a strong `AGENT_TOKEN`:**

```bash
# Linux / macOS
openssl rand -hex 32
```

Use the same value in both `miniobserv` and `miniobserv-agent`. The token is a shared HS256 secret — it never leaves your infrastructure.

Open `http://localhost:8080` after `docker compose up`. The dashboard appears as soon as the agent starts pushing data (within ~10 seconds).

---

## 3. Express.js Integration

### 3a. Write logs to a file (so MiniObserv can tail them)

MiniObserv's agent watches files listed in `LOG_PATHS` and streams every new line into the dashboard log viewer. The simplest approach is to add a file transport to your existing Winston logger:

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

Mount the same volume in your app container so the agent can read the file:

```yaml
services:
  api:
    volumes:
      - app_logs:/var/log/myapp
```

### 3b. Request logging middleware (structured format)

Log every request so MiniObserv's log stream shows meaningful HTTP activity. Log level is determined by status code — ERROR for 5xx, WARN for 4xx, INFO otherwise:

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

This is enough to see HTTP errors and slow routes in the dashboard without any SDK.

### 3c. Pushing custom app metrics with the SDK (optional)

Install the SDK:

```bash
npm install @kamerrezz/miniobserv
```

Initialize the client once and reuse it across your app:

```javascript
import { MiniObservClient } from '@kamerrezz/miniobserv';

const obs = new MiniObservClient({
  baseUrl: process.env.MINIOBSERV_URL ?? 'http://miniobserv:8080',
  agentToken: process.env.AGENT_TOKEN,
  defaultHost: process.env.HOSTNAME ?? 'api-server',
});
```

Push a custom signal from middleware — fire-and-forget, never block the request:

```javascript
app.use((req, res, next) => {
  const start = Date.now();
  res.on('finish', () => {
    const heapUsed = process.memoryUsage().heapUsed;
    const heapTotal = process.memoryUsage().heapTotal;

    obs.pushMetric('mem.used_pct', (heapUsed / heapTotal) * 100)
      .catch(() => {}); // never let observability crash the request
  });
  next();
});
```

**A note on metric names:** MiniObserv has a fixed set of canonical names (`cpu.usage_pct`, `mem.used_pct`, `disk.used_pct`, etc.). The agent already populates these from the OS. When pushing from the SDK, use the same names to add data points from the app's perspective — or push any value you want to track. The recommended approach for most apps is to start with the sidecar pattern (agent only), and add SDK pushes only for high-value signals like error rate or DB latency.

---

## 4. NestJS Integration

### 4a. Global interceptor for request logging

Create an interceptor that logs every request with method, URL, status, and duration:

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

### 4b. Register globally in `main.ts`

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

### 4c. Route NestJS logs to a file with Winston

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

With this in place, every NestJS log — including the interceptor output — lands in `/var/log/myapp/app.log`, which the MiniObserv agent tails automatically.

---

## 5. Next.js Integration

### 5a. API route middleware (App Router, Next.js 14+)

Next.js middleware runs at the Edge and does not have access to Node.js file APIs. The simplest way to get logs into MiniObserv is to write structured JSON to stdout and redirect Docker's output to a file.

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

### 5b. Redirect Next.js stdout to a file (docker-compose)

Pipe the container's stdout to a file that the agent can tail:

```yaml
services:
  nextjs:
    command: sh -c "node server.js 2>&1 | tee /var/log/myapp/app.log"
    volumes:
      - app_logs:/var/log/myapp
```

The `tee` command writes to both stdout (so you can still `docker logs`) and the file. The MiniObserv agent picks up new lines automatically via `LOG_PATHS`.

---

## 6. Alert Rules for Node.js Apps

Define alert rules as a JSON array in the `ALERT_RULES` environment variable on the MiniObserv server. Each rule fires when the condition holds for the specified duration:

```json
[
  { "host": "*", "name": "cpu.usage_pct",  "op": ">", "threshold": 80, "for": "5m"  },
  { "host": "*", "name": "mem.used_pct",   "op": ">", "threshold": 85, "for": "10m" },
  { "host": "*", "name": "disk.used_pct",  "op": ">", "threshold": 90, "for": "1m"  }
]
```

- `host: "*"` matches all hosts reporting to this server
- `for` is the minimum sustained duration before the alert fires — avoids noise from transient spikes
- Fired alerts appear in the dashboard; webhook delivery is not yet implemented

A practical starting point for a typical Node.js API:

| Metric | Threshold | For | Rationale |
|---|---|---|---|
| `cpu.usage_pct` | 80% | 5m | Sustained load — not a single spike |
| `mem.used_pct` | 85% | 10m | Memory leaks grow slowly |
| `disk.used_pct` | 90% | 1m | Disk full is immediate and fatal |

---

## 7. What You Get Without Writing a Single Line of SDK Code

If you only add the two MiniObserv services to your docker-compose and point `LOG_PATHS` at your app's log file, you get:

- **System metrics** — CPU, memory, disk, network collected every 10 seconds, automatically
- **Log stream** — every line your app writes to the log file, visible in the dashboard with timestamps
- **Alerts** — notifications when resources cross the thresholds you define
- **Live dashboard** — sparkline charts at `http://localhost:8080`, no setup required

The SDK is for when you need to go further: track error rates by route, measure DB query latency, record queue depths. Start without it. Add it when you have a specific signal you cannot observe from logs and system metrics alone.

---

## Reference

### Canonical metric names

| Name | Unit | Description |
|---|---|---|
| `cpu.usage_pct` | % | CPU utilization across all cores |
| `mem.used_pct` | % | Memory used as a percentage of total |
| `mem.used_bytes` | bytes | Memory currently in use |
| `mem.total_bytes` | bytes | Total installed memory |
| `disk.used_pct` | % | Disk used percentage (mount `/`) |
| `disk.used_bytes` | bytes | Disk space in use |
| `disk.total_bytes` | bytes | Total disk capacity |
| `net.bytes_in` | bytes | Network bytes received (delta per tick) |
| `net.bytes_out` | bytes | Network bytes sent (delta per tick) |

> `net.*` metrics are delta values — no data is emitted on the first collection tick.

### Agent environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `SERVER_URL` | yes | — | Base URL of the MiniObserv server |
| `AGENT_TOKEN` | yes | — | Shared HS256 secret (min 16 chars) |
| `COLLECT_INTERVAL` | no | `10s` | Collection frequency (1s–300s) |
| `LOG_PATHS` | no | — | Comma-separated paths to tail |
| `AGENT_HOST` | no | OS hostname | Host label on all metrics |
