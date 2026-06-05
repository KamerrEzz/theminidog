CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

CREATE TABLE IF NOT EXISTS metrics (
    time   TIMESTAMPTZ      NOT NULL,
    host   TEXT             NOT NULL,
    name   TEXT             NOT NULL,
    value  DOUBLE PRECISION NOT NULL,
    labels JSONB
);

SELECT create_hypertable('metrics', 'time',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

CREATE INDEX IF NOT EXISTS idx_metrics_host_name_time
    ON metrics (host, name, time DESC);
