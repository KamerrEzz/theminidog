import { MiniObservClient } from '../src/index.js';

const client = new MiniObservClient({
  baseUrl: process.env['MINIOBSERV_URL'] ?? 'http://localhost:8080',
  agentToken: process.env['AGENT_TOKEN'] ?? 'your-secret-here-min-16-chars',
  defaultHost: 'my-app-server',
});

async function main() {
  // Check server is up
  const ready = await client.readyz();
  if (!ready) {
    console.error('Server not ready');
    process.exit(1);
  }

  // Push a single metric
  await client.pushMetric('cpu.usage_pct', 42.5, { core: 'total' });
  console.log('Pushed cpu.usage_pct');

  // Push a batch of metrics
  const result = await client.pushMetrics({
    host: 'my-app-server',
    metrics: [
      {
        time: new Date().toISOString(),
        host: 'my-app-server',
        name: 'mem.used_pct',
        value: 68.2,
      },
      {
        time: new Date().toISOString(),
        host: 'my-app-server',
        name: 'disk.used_pct',
        value: 54.1,
        labels: { mount: '/data' },
      },
    ],
  });
  console.log(`Ingested ${result.ingested} metrics`);
}

main().catch(console.error);
