-- Remove retention policies
SELECT remove_retention_policy('metrics', if_exists => true);
SELECT remove_retention_policy('logs', if_exists => true);

-- Remove compression policies
SELECT remove_compression_policy('metrics', if_exists => true);
SELECT remove_compression_policy('logs', if_exists => true);

-- Disable compression settings
ALTER TABLE metrics RESET (timescaledb.compress);
ALTER TABLE logs RESET (timescaledb.compress);

-- Note: hypertable conversion of 'logs' is irreversible without data loss.
-- The composite PRIMARY KEY (id, time) on logs also remains.
-- To fully revert, restore from backup and re-run migrations 001 and 002.
