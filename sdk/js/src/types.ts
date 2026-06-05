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
