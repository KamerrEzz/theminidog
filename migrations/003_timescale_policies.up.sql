-- Step 1: Rebuild logs primary key to include the partition column (ADR-11)
-- TimescaleDB requires the partitioning column to be part of any UNIQUE/PK constraint.
ALTER TABLE logs DROP CONSTRAINT IF EXISTS logs_pkey;
ALTER TABLE logs ADD PRIMARY KEY (id, time);

-- Step 2: Convert logs to a hypertable (logs was a plain BIGSERIAL table)
SELECT create_hypertable('logs', 'time', migrate_data => true, if_not_exists => true);

-- Step 3: Enable compression on metrics (already a hypertable from migration 001)
ALTER TABLE metrics SET (
    timescaledb.compress,
    timescaledb.compress_orderby = 'time DESC'
);

-- Step 4: Enable compression on logs
ALTER TABLE logs SET (
    timescaledb.compress,
    timescaledb.compress_orderby = 'time DESC'
);

-- Step 5: Add compression policies (compress chunks older than 7 days)
SELECT add_compression_policy('metrics', INTERVAL '7 days');
SELECT add_compression_policy('logs', INTERVAL '7 days');

-- Step 6: Add retention policies (drop chunks older than threshold)
SELECT add_retention_policy('metrics', INTERVAL '30 days');
SELECT add_retention_policy('logs', INTERVAL '14 days');
