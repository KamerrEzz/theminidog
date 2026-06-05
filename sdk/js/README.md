# @miniobserv/sdk

JavaScript/TypeScript SDK for [MiniObserv](https://github.com/your-org/theminidog) — a lightweight metrics ingestion and query server.

- Zero runtime dependencies (Node.js built-ins only: `node:crypto` + native `fetch`)
- Full TypeScript types
- Automatic JWT minting, caching, and refresh
- Node.js 18+

## Installation

```bash
npm install @miniobserv/sdk
# or
yarn add @miniobserv/sdk
# or
pnpm add @miniobserv/sdk
```

## Quick start

```typescript
import { MiniObservClient } from '@miniobserv/sdk';

const client = new MiniObservClient({
  baseUrl: 'http://localhost:8080',
  agentToken: process.env.AGENT_TOKEN!,
});

await client.pushMetric('cpu.usage_pct', 72.4);
```

That is it. The SDK mints, caches, and refreshes the JWT automatically — you never touch tokens directly.

## JWT handling

Authentication uses HS256 JWTs signed with your `AGENT_TOKEN` secret. The SDK:

1. Mints a token with a **24-hour TTL** on the first authenticated request.
2. **Caches** the token in memory for the lifetime of the client instance.
3. **Refreshes** it automatically **5 minutes before expiry** — no stale-token errors in long-running processes.

The signing is done entirely with `node:crypto`'s `createHmac` — no `jsonwebtoken` or any other package required.

## API reference

### `new MiniObservClient(options)`

| Option | Type | Required | Description |
|---|---|---|---|
| `baseUrl` | `string` | Yes | Server base URL, e.g. `http://localhost:8080` |
| `agentToken` | `string` | Yes | HS256 signing secret (`AGENT_TOKEN`) |
| `defaultHost` | `string` | No | Host label used by `pushMetric()`. Defaults to `"sdk-client"` |

### `client.pushMetric(name, value, labels?)`

Convenience method. Pushes a single metric using the current timestamp and `defaultHost`.

```typescript
await client.pushMetric('mem.used_pct', 68.2);
await client.pushMetric('disk.used_pct', 54.1, { mount: '/data' });
```

Returns `Promise<IngestResponse>` — `{ ingested: number }`.

### `client.pushMetrics(batch)`

Push an explicit batch with full control over host, timestamp, and labels.

```typescript
await client.pushMetrics({
  host: 'web-01',
  metrics: [
    { time: new Date().toISOString(), host: 'web-01', name: 'cpu.usage_pct', value: 42.5 },
    { time: new Date().toISOString(), host: 'web-01', name: 'mem.used_pct',  value: 68.2 },
  ],
});
```

Returns `Promise<IngestResponse>`.

### `client.queryMetrics(options)`

Query a time-bucketed metric series.

```typescript
const result = await client.queryMetrics({
  host: 'web-01',
  name: 'cpu.usage_pct',
  from: new Date(Date.now() - 3600_000),
  to:   new Date(),
  bucket: '5m',
  agg:    'avg',
});

for (const point of result.points) {
  console.log(point.time, point.value);
}
```

| Option | Type | Required | Description |
|---|---|---|---|
| `host` | `string` | Yes | Host to query |
| `name` | `MetricName` | Yes | Canonical metric name |
| `from` | `Date \| string` | Yes | Start of range (ISO 8601 or `Date`) |
| `to` | `Date \| string` | Yes | End of range (ISO 8601 or `Date`) |
| `bucket` | `BucketInterval` | No | `1m` `5m` `15m` `1h` `1d` |
| `agg` | `AggFunction` | No | `avg` `max` `min` |

Returns `Promise<QueryResponse>` — `{ host, name, bucket, agg, points: [{time, value}] }`.

### `client.healthz()`

Returns `Promise<boolean>` — `true` if the server responds to `GET /healthz`. No authentication required.

### `client.readyz()`

Returns `Promise<boolean>` — `true` if the server responds to `GET /readyz` (database connectivity included). No authentication required.

### `MiniObservError`

Thrown on any non-2xx HTTP response.

```typescript
import { MiniObservClient, MiniObservError } from '@miniobserv/sdk';

try {
  await client.pushMetric('cpu.usage_pct', 72.4);
} catch (err) {
  if (err instanceof MiniObservError) {
    console.error(err.status, err.message); // e.g. 401, "MiniObserv API error 401: unauthorized"
  }
}
```

## Canonical metric names

| Name | Description |
|---|---|
| `cpu.usage_pct` | CPU utilization percentage |
| `mem.used_pct` | Memory used percentage |
| `mem.used_bytes` | Memory used in bytes |
| `mem.total_bytes` | Total memory in bytes |
| `disk.used_pct` | Disk used percentage |
| `disk.used_bytes` | Disk used in bytes |
| `disk.total_bytes` | Total disk capacity in bytes |
| `net.bytes_in` | Network bytes received |
| `net.bytes_out` | Network bytes sent |

## Examples

```bash
# Push metrics
AGENT_TOKEN=your-secret MINIOBSERV_URL=http://localhost:8080 npx tsx examples/push-metrics.ts

# Query metrics
AGENT_TOKEN=your-secret HOST=web-01 npx tsx examples/query-metrics.ts
```

## TypeScript usage

All types are exported from the package root:

```typescript
import type {
  Metric,
  MetricName,
  MetricBatch,
  BucketInterval,
  AggFunction,
  IngestResponse,
  QueryOptions,
  QueryResponse,
  QueryPoint,
  MiniObservClientOptions,
} from '@miniobserv/sdk';
```

## Build

```bash
npm install
npm run build   # tsc → dist/
```

The compiled output lands in `dist/` with `.d.ts` declarations and source maps.
