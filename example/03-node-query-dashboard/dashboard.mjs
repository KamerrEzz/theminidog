/**
 * Terminal ASCII dashboard — refreshes every 10 seconds.
 * Queries cpu.usage_pct and mem.used_pct for the last hour and renders
 * a horizontal bar chart in the terminal.
 *
 * Required environment variables:
 *   MINIOBSERV_URL  — e.g. http://localhost:8080
 *   AGENT_TOKEN     — HS256 signing secret
 *   METRIC_HOST     — host to query (default: "node-example")
 */

import { MiniObservClient } from '@kamerrezz/miniobserv';

const MINIOBSERV_URL = process.env.MINIOBSERV_URL;
const AGENT_TOKEN    = process.env.AGENT_TOKEN;
const METRIC_HOST    = process.env.METRIC_HOST ?? 'node-example';
const REFRESH_MS     = 10_000;
const BAR_WIDTH      = 40; // characters for 100%

if (!MINIOBSERV_URL) { console.error('ERROR: MINIOBSERV_URL is not set'); process.exit(1); }
if (!AGENT_TOKEN)    { console.error('ERROR: AGENT_TOKEN is not set');    process.exit(1); }

const client = new MiniObservClient({
  baseUrl: MINIOBSERV_URL,
  agentToken: AGENT_TOKEN,
  defaultHost: METRIC_HOST,
});

// ── helpers ───────────────────────────────────────────────────────────────────

function bar(pct, width = BAR_WIDTH) {
  const filled = Math.round((Math.min(Math.max(pct, 0), 100) / 100) * width);
  return '█'.repeat(filled) + '░'.repeat(width - filled);
}

function colorPct(pct) {
  if (pct >= 80) return `\x1b[31m${pct.toFixed(1).padStart(5)}%\x1b[0m`; // red
  if (pct >= 60) return `\x1b[33m${pct.toFixed(1).padStart(5)}%\x1b[0m`; // yellow
  return `\x1b[32m${pct.toFixed(1).padStart(5)}%\x1b[0m`;                 // green
}

function shortTime(iso) {
  return iso.slice(11, 16); // HH:MM
}

function renderChart(label, points, unit = '%') {
  const lines = [];
  lines.push(`  \x1b[1m${label}\x1b[0m`);
  if (!points || points.length === 0) {
    lines.push('  (no data)');
    return lines;
  }
  // Show last 12 points so the chart fits most terminals
  const visible = points.slice(-12);
  for (const pt of visible) {
    const v = pt.value;
    const b = unit === '%' ? bar(v) : bar((v / (10 * 1024 ** 3)) * 100); // scale bytes to 10 GB
    const label2 = unit === '%' ? colorPct(v) : `\x1b[36m${(v / 1024 ** 3).toFixed(2).padStart(6)} GB\x1b[0m`;
    lines.push(`  ${shortTime(pt.time)} │${b}│ ${label2}`);
  }
  return lines;
}

// ── main render loop ──────────────────────────────────────────────────────────

async function render() {
  const now  = new Date();
  const from = new Date(now - 60 * 60 * 1000); // last hour

  let cpuPoints = [];
  let memPoints = [];

  try {
    const [cpuResp, memResp] = await Promise.all([
      client.queryMetrics({ host: METRIC_HOST, name: 'cpu.usage_pct', from, to: now, bucket: '5m', agg: 'avg' }),
      client.queryMetrics({ host: METRIC_HOST, name: 'mem.used_pct',  from, to: now, bucket: '5m', agg: 'avg' }),
    ]);
    cpuPoints = cpuResp.points ?? [];
    memPoints = memResp.points ?? [];
  } catch (err) {
    process.stdout.write(`\x1b[2J\x1b[H`); // clear
    console.error(`Query error: ${err.message}`);
    return;
  }

  // Clear terminal and redraw
  process.stdout.write('\x1b[2J\x1b[H');

  const header = `MiniObserv Dashboard   host: ${METRIC_HOST}   refreshed: ${now.toISOString().slice(0, 19)}Z`;
  console.log(`\x1b[1;34m${header}\x1b[0m`);
  console.log(`\x1b[34m${'─'.repeat(header.length)}\x1b[0m\n`);

  const cpuLines = renderChart('CPU Usage  (5-min avg, last hour)', cpuPoints, '%');
  const memLines = renderChart('Memory Used  (5-min avg, last hour)', memPoints, '%');

  cpuLines.forEach(l => console.log(l));
  console.log();
  memLines.forEach(l => console.log(l));

  console.log(`\n\x1b[2m  Next refresh in ${REFRESH_MS / 1000}s — Press Ctrl+C to quit\x1b[0m`);
}

console.log('MiniObserv ASCII Dashboard starting...');
await render();
setInterval(async () => {
  try { await render(); } catch (err) { console.error('Render error:', err.message); }
}, REFRESH_MS);
