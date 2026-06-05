export interface Metric {
  time: Date | string;
  host: string;
  name: MetricName;
  value: number;
  labels?: Record<string, string>;
}

export type MetricName =
  | 'cpu.usage_pct'
  | 'mem.used_pct'
  | 'mem.used_bytes'
  | 'mem.total_bytes'
  | 'disk.used_pct'
  | 'disk.used_bytes'
  | 'disk.total_bytes'
  | 'net.bytes_in'
  | 'net.bytes_out';

export type BucketInterval = '1m' | '5m' | '15m' | '1h' | '1d';
export type AggFunction = 'avg' | 'max' | 'min';

export interface MetricBatch {
  host: string;
  metrics: Metric[];
}

export interface IngestResponse {
  ingested: number;
}

export interface QueryPoint {
  time: string;
  value: number;
}

export interface QueryResponse {
  host: string;
  name: string;
  bucket: string;
  agg: string;
  points: QueryPoint[];
}

export interface QueryOptions {
  host: string;
  name: MetricName;
  from: Date | string;
  to: Date | string;
  bucket?: BucketInterval;
  agg?: AggFunction;
}

export interface MiniObservClientOptions {
  baseUrl: string;
  agentToken: string; // HS256 signing secret
  defaultHost?: string;
}

// ── Alerts ────────────────────────────────────────────────────────────────────

export interface AlertRule {
  Host: string;
  Name: MetricName | 'host.down' | string;
  Op: '>' | '<';
  Threshold: number;
  For: string; // e.g. "5m"
}

export type AlertState = 'ok' | 'pending' | 'firing';

export interface Alert {
  rule: AlertRule;
  state: AlertState;
  value: number;
  updated_at: string; // ISO 8601
}

export interface AlertsResponse {
  alerts: Alert[];
}

// ── Hosts ─────────────────────────────────────────────────────────────────────

export type HostHealthStatus = 'ok' | 'stale' | 'down';

export interface HostStatus {
  host: string;
  status: HostHealthStatus;
  last_seen: string; // ISO 8601
}

export interface HostsResponse {
  hosts: HostStatus[];
}

// ── Logs ──────────────────────────────────────────────────────────────────────

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

export interface LogEntry {
  id: number;
  time: string;
  host: string;
  level: LogLevel;
  message: string;
  source?: string;
}

export interface LogQueryOptions {
  host?: string;
  level?: LogLevel;
  from?: Date | string;
  to?: Date | string;
  search?: string;
  limit?: number;
  cursor?: number; // keyset pagination: last seen id
}

export interface LogQueryResponse {
  entries: LogEntry[];
  next_cursor?: number;
}

export interface LogBatch {
  host: string;
  entries: LogEntryInput[];
}

export interface LogEntryInput {
  time: Date | string;
  host: string;
  level: LogLevel;
  message: string;
  source?: string;
}
