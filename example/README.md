# MiniObserv Examples

Practical examples showing how to interact with the MiniObserv API from different environments.

Every example uses these environment variables — set them before running:

```sh
export MINIOBSERV_URL=http://localhost:8080
export AGENT_TOKEN=your-secret-here-min-16-chars
```

---

## Examples

| # | Directory | What it does |
|---|-----------|--------------|
| 01 | [01-curl-quickstart](./01-curl-quickstart/) | Shell script: health check → mint JWT → push metrics → query — full lifecycle with `curl` and Python |
| 02 | [02-node-push-metrics](./02-node-push-metrics/) | Node.js loop that pushes 5 metric types every 5 seconds using the `@kamerrezz/miniobserv` SDK |
| 03 | [03-node-query-dashboard](./03-node-query-dashboard/) | Node.js terminal dashboard that queries cpu + memory and renders an ASCII bar chart every 10 seconds |
| 04 | [04-go-http-client](./04-go-http-client/) | Pure Go (stdlib only) client that mints a JWT manually, pushes a metric, and queries it back |
| 05 | [05-web-dashboard](./05-web-dashboard/) | Standalone HTML page — form + SVG line chart, calls the API via `fetch`, zero dependencies, no build step |

---

## Prerequisites by example

| Example | Requires |
|---------|----------|
| 01 | bash, curl, python3 |
| 02 | Node.js >= 18 |
| 03 | Node.js >= 18 |
| 04 | Go >= 1.21 |
| 05 | Any modern browser |

All examples assume a running MiniObserv server at `MINIOBSERV_URL`.
