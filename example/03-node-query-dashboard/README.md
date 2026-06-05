# 03 — Node.js Query Dashboard

Terminal ASCII dashboard that queries `cpu.usage_pct` and `mem.used_pct` for the last hour and renders a horizontal bar chart. Refreshes every 10 seconds.

## What it does

- Queries two metrics from MiniObserv every 10 seconds (5-minute buckets, average aggregation)
- Renders a color-coded bar chart in the terminal (green → yellow → red by severity)
- Shows the last 12 data points per metric (fits most terminal widths)
- Clears and redraws the screen on each refresh

## Prerequisites

- Node.js >= 18
- SDK built: `cd ../../sdk/js && npm install && npm run build`
- A running MiniObserv server with some data (run example 02 first to generate data)

## How to run

```sh
export MINIOBSERV_URL=http://localhost:8080
export AGENT_TOKEN=your-secret-here-min-16-chars
export METRIC_HOST=node-example
npm install
node dashboard.mjs
```
