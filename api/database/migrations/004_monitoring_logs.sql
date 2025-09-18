-- Scan logs and monitoring (matches Go usage)
CREATE TABLE IF NOT EXISTS scan_logs (
    id BIGSERIAL PRIMARY KEY,
    host_id BIGINT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    level TEXT NOT NULL DEFAULT 'info',
    message TEXT NOT NULL,
    data JSONB NOT NULL DEFAULT '{}'
);

-- Indexes for log queries
CREATE INDEX IF NOT EXISTS idx_scan_logs_host_at ON scan_logs(host_id, at DESC);