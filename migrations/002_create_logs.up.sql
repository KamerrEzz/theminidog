CREATE TABLE IF NOT EXISTS logs (
    id      BIGSERIAL   PRIMARY KEY,
    time    TIMESTAMPTZ NOT NULL,
    host    TEXT        NOT NULL,
    path    TEXT        NOT NULL,
    level   TEXT        NOT NULL CHECK (level IN ('info', 'warn', 'error', 'debug')),
    message TEXT        NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_logs_host_time
    ON logs (host, time DESC);

CREATE INDEX IF NOT EXISTS idx_logs_level_time
    ON logs (level, time DESC);

CREATE INDEX IF NOT EXISTS idx_logs_time
    ON logs (time DESC);
