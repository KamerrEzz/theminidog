# 05 — Web Dashboard

Standalone HTML page that queries MiniObserv and renders a SVG line chart. Zero dependencies, zero build step — open the file directly in any modern browser.

## What it does

- Form: server URL, agent token (signing secret), host, metric name, time range, bucket, aggregation
- Mints a HS256 JWT in the browser using the Web Crypto API (`crypto.subtle`)
- Calls `GET /api/v1/metrics/query` via `fetch` (CORS must be enabled on the server)
- Renders an SVG line chart with area fill, hover tooltips, and axis labels
- Formats values intelligently: `%` for `*_pct` metrics, human-readable bytes for `*_bytes` metrics
- Shows the raw JSON response below the chart

## Prerequisites

- A running MiniObserv server (CORS headers must allow browser origin)
- A modern browser (Chrome, Firefox, Safari, Edge — Web Crypto is required)
- No Node.js, no build step, no package install

## How to run

```sh
# Just open the file in your browser:
open example/05-web-dashboard/index.html
# or on Windows:
start example/05-web-dashboard/index.html
```

Then fill in the form:
- **Server URL**: `http://localhost:8080`
- **Agent Token**: your `AGENT_TOKEN` secret
- **Host**: the host you pushed data from (e.g. `node-example`)

> Note: browsers block `fetch` calls to `http://` from a `file://` page on some
> setups. If you hit a CORS or mixed-content error, serve the file over HTTP:
> `python3 -m http.server 3000` from the `05-web-dashboard/` directory, then
> open `http://localhost:3000`.
