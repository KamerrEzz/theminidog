import { MiniObservClient } from '@kamerrezz/miniobserv'

const client = new MiniObservClient({
  baseUrl: process.env.MINIOBSERV_URL ?? 'http://localhost:8080',
  agentToken: process.env.MINIOBSERV_TOKEN ?? 'dev-secret',
  defaultHost: process.env.APP_HOST ?? 'tasks-api',
})

export const obs = {
  info: (message: string, source?: string) =>
    client.pushLog('info', message, { source }).catch(() => {}),

  error: (message: string, source?: string) =>
    client.pushLog('error', message, { source }).catch(() => {}),

  warn: (message: string, source?: string) =>
    client.pushLog('warn', message, { source }).catch(() => {}),

  request: (method: string, path: string, status: number, ms: number) =>
    client
      .pushLog(
        status >= 500 ? 'error' : status >= 400 ? 'warn' : 'info',
        `${method} ${path} ${status} ${ms}ms`,
        { source: 'http' }
      )
      .catch(() => {}),
}
