import { MiniObservClient } from '../src/index.js';

const client = new MiniObservClient({
  baseUrl: process.env['MINIOBSERV_URL'] ?? 'http://localhost:8080',
  agentToken: process.env['AGENT_TOKEN'] ?? 'your-secret-here-min-16-chars',
});

async function main() {
  const now = new Date();
  const oneHourAgo = new Date(now.getTime() - 60 * 60 * 1000);

  const result = await client.queryMetrics({
    host: process.env['HOST'] ?? 'my-host',
    name: 'cpu.usage_pct',
    from: oneHourAgo,
    to: now,
    bucket: '5m',
    agg: 'avg',
  });

  console.log(`CPU usage for ${result.host} — last hour (5m avg):`);
  for (const point of result.points) {
    const time = new Date(point.time).toLocaleTimeString();
    const bar = '█'.repeat(Math.round(point.value / 5));
    console.log(`  ${time}  ${bar} ${point.value.toFixed(1)}%`);
  }
}

main().catch(console.error);
