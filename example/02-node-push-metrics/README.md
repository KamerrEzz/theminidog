# 02 — Node.js Push Metrics

Continuously pushes synthetic data for 5 different metric types to the MiniObserv server every 5 seconds. Uses the `@kamerrezz/miniobserv` SDK from the local monorepo.

## What it does

- Connects to MiniObserv using the SDK client
- Every 5 seconds generates plausible random values for:
  - `cpu.usage_pct`, `mem.used_pct`, `mem.used_bytes`, `disk.used_pct`, `net.bytes_in`
- Pushes them as a single batch and logs the confirmation

## Prerequisites

- Node.js >= 18
- `npm install` run inside `sdk/js/` (the SDK must be built: `npm run build`)
- A running MiniObserv server

## How to run

```sh
export MINIOBSERV_URL=http://localhost:8080
export AGENT_TOKEN=your-secret-here-min-16-chars
npm install
node index.mjs
```

Optional: override the host label with `METRIC_HOST=my-machine`.
