/**
 * Pushes a batch of 5 different metric types every 5 seconds.
 *
 * Required environment variables:
 *   MINIOBSERV_URL  — e.g. http://localhost:8080
 *   AGENT_TOKEN     — HS256 signing secret (min 16 chars)
 *
 * Optional:
 *   METRIC_HOST     — hostname label for the metrics (default: "node-example")
 */

import { MiniObservClient } from '@kamerrezz/miniobserv';

const MINIOBSERV_URL = process.env.MINIOBSERV_URL;
const AGENT_TOKEN    = process.env.AGENT_TOKEN;
const METRIC_HOST    = process.env.METRIC_HOST ?? 'node-example';
const INTERVAL_MS    = 5_000;

if (!MINIOBSERV_URL) {
  console.error('ERROR: MINIOBSERV_URL is not set');
  process.exit(1);
}
if (!AGENT_TOKEN) {
  console.error('ERROR: AGENT_TOKEN is not set');
  process.exit(1);
}

const client = new MiniObservClient({
  baseUrl: MINIOBSERV_URL,
  agentToken: AGENT_TOKEN,
  defaultHost: METRIC_HOST,
});

/** Generate plausible synthetic metric values. */
function syntheticMetrics() {
  const now = new Date().toISOString();
  return [
    { time: now, host: METRIC_HOST, name: 'cpu.usage_pct',  value: 10 + Math.random() * 80 },
    { time: now, host: METRIC_HOST, name: 'mem.used_pct',   value: 30 + Math.random() * 50 },
    { time: now, host: METRIC_HOST, name: 'mem.used_bytes', value: Math.floor((3 + Math.random() * 5) * 1024 ** 3) },
    { time: now, host: METRIC_HOST, name: 'disk.used_pct',  value: 40 + Math.random() * 40 },
    { time: now, host: METRIC_HOST, name: 'net.bytes_in',   value: Math.floor(Math.random() * 10 * 1024 ** 2) },
  ];
}

async function pushOnce() {
  const metrics = syntheticMetrics();
  const result = await client.pushMetrics({ host: METRIC_HOST, metrics });
  const ts = new Date().toISOString();
  console.log(`[${ts}] Pushed ${result.ingested} metrics for host="${METRIC_HOST}"`);
  metrics.forEach(m => {
    console.log(`         ${m.name.padEnd(18)} = ${m.value.toFixed(2)}`);
  });
}

console.log(`MiniObserv push loop started — interval ${INTERVAL_MS / 1000}s — host "${METRIC_HOST}"`);
console.log(`Server: ${MINIOBSERV_URL}`);
console.log('Press Ctrl+C to stop.\n');

// Push immediately on start, then on each interval
await pushOnce();
setInterval(async () => {
  try {
    await pushOnce();
  } catch (err) {
    console.error(`[${new Date().toISOString()}] Push failed:`, err.message);
  }
}, INTERVAL_MS);
