import { mintAgentToken } from './jwt.js';
import type {
  MiniObservClientOptions,
  MetricBatch,
  IngestResponse,
  QueryOptions,
  QueryResponse,
  MetricName,
  AlertsResponse,
  HostsResponse,
  LogQueryOptions,
  LogQueryResponse,
} from './types.js';

export class MiniObservClient {
  private baseUrl: string;
  private secret: string;
  private defaultHost: string;
  private cachedToken: string | null = null;
  private tokenExpiry: number = 0;

  constructor(options: MiniObservClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/$/, '');
    this.secret = options.agentToken;
    this.defaultHost = options.defaultHost ?? 'sdk-client';
  }

  private getToken(): string {
    const now = Math.floor(Date.now() / 1000);
    // Refresh 5 minutes before expiry
    if (!this.cachedToken || now >= this.tokenExpiry - 300) {
      this.cachedToken = mintAgentToken(this.secret);
      this.tokenExpiry = now + 86400;
    }
    return this.cachedToken;
  }

  private async request<T>(
    method: string,
    path: string,
    options?: {
      body?: unknown;
      params?: Record<string, string | undefined>;
      auth?: boolean;
    }
  ): Promise<T> {
    const url = new URL(`${this.baseUrl}${path}`);
    if (options?.params) {
      for (const [k, v] of Object.entries(options.params)) {
        if (v !== undefined) url.searchParams.set(k, v);
      }
    }

    const headers: Record<string, string> = {};
    if (options?.auth !== false) {
      headers['Authorization'] = `Bearer ${this.getToken()}`;
    }
    if (options?.body !== undefined) {
      headers['Content-Type'] = 'application/json';
    }

    const res = await fetch(url.toString(), {
      method,
      headers,
      body: options?.body !== undefined ? JSON.stringify(options.body) : undefined,
    });

    if (!res.ok) {
      const err = await res
        .json()
        .catch(() => ({ error: res.statusText })) as { error: string };
      throw new MiniObservError(res.status, err.error ?? res.statusText);
    }

    return res.json() as Promise<T>;
  }

  /** Push a batch of metrics to the server. */
  async pushMetrics(batch: MetricBatch): Promise<IngestResponse> {
    return this.request<IngestResponse>('POST', '/api/v1/metrics', { body: batch });
  }

  /** Push a single metric (convenience wrapper). */
  async pushMetric(
    name: MetricName,
    value: number,
    labels?: Record<string, string>
  ): Promise<IngestResponse> {
    const host = this.defaultHost;
    return this.pushMetrics({
      host,
      metrics: [
        {
          time: new Date().toISOString(),
          host,
          name,
          value,
          labels,
        },
      ],
    });
  }

  /** Query time-bucketed metric series. */
  async queryMetrics(options: QueryOptions): Promise<QueryResponse> {
    return this.request<QueryResponse>('GET', '/api/v1/metrics/query', {
      params: {
        host: options.host,
        name: options.name,
        from:
          options.from instanceof Date
            ? options.from.toISOString()
            : options.from,
        to:
          options.to instanceof Date
            ? options.to.toISOString()
            : options.to,
        bucket: options.bucket,
        agg: options.agg,
      },
    });
  }

  /** Get all active and resolved alerts. No authentication required. */
  async getAlerts(): Promise<AlertsResponse> {
    return this.request<AlertsResponse>('GET', '/api/v1/alerts', { auth: false });
  }

  /** Get health status for all known hosts. No authentication required. */
  async getHosts(): Promise<HostsResponse> {
    return this.request<HostsResponse>('GET', '/api/v1/hosts', { auth: false });
  }

  /** Query log entries with optional filtering and keyset pagination. */
  async queryLogs(options?: LogQueryOptions): Promise<LogQueryResponse> {
    return this.request<LogQueryResponse>('GET', '/api/v1/logs/query', {
      params: {
        host: options?.host,
        level: options?.level,
        from: options?.from instanceof Date ? options.from.toISOString() : options?.from,
        to: options?.to instanceof Date ? options.to.toISOString() : options?.to,
        search: options?.search,
        limit: options?.limit?.toString(),
        cursor: options?.cursor?.toString(),
      },
    });
  }

  /** Check server liveness. */
  async healthz(): Promise<boolean> {
    try {
      await this.request<string>('GET', '/healthz', { auth: false });
      return true;
    } catch {
      return false;
    }
  }

  /** Check server readiness (includes DB check). */
  async readyz(): Promise<boolean> {
    try {
      await this.request<string>('GET', '/readyz', { auth: false });
      return true;
    } catch {
      return false;
    }
  }
}

export class MiniObservError extends Error {
  constructor(
    public readonly status: number,
    message: string
  ) {
    super(`MiniObserv API error ${status}: ${message}`);
    this.name = 'MiniObservError';
  }
}
