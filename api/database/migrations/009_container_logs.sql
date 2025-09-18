-- Create table for centralized log storage
CREATE TABLE IF NOT EXISTS container_logs (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    hostname VARCHAR(255) NOT NULL,
    stack_name VARCHAR(255),
    service_name VARCHAR(255) NOT NULL,
    container_id VARCHAR(64) NOT NULL,
    container_name VARCHAR(255),
    level VARCHAR(10) DEFAULT 'INFO',
    source VARCHAR(10) DEFAULT 'stdout', -- stdout, stderr
    message TEXT NOT NULL,
    labels JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_container_logs_timestamp ON container_logs (timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_container_logs_hostname ON container_logs (hostname);
CREATE INDEX IF NOT EXISTS idx_container_logs_stack ON container_logs (stack_name);
CREATE INDEX IF NOT EXISTS idx_container_logs_service ON container_logs (service_name);
CREATE INDEX IF NOT EXISTS idx_container_logs_container ON container_logs (container_name);
CREATE INDEX IF NOT EXISTS idx_container_logs_level ON container_logs (level);
CREATE INDEX IF NOT EXISTS idx_container_logs_composite ON container_logs (hostname, stack_name, timestamp DESC);

-- Add GIN index for full-text search on message
CREATE INDEX IF NOT EXISTS idx_container_logs_message_gin ON container_logs USING GIN (to_tsvector('english', message));

-- Function to automatically prune old logs (optional, can be called periodically)
CREATE OR REPLACE FUNCTION prune_old_logs(retention_days INTEGER DEFAULT 7)
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM container_logs
    WHERE timestamp < NOW() - INTERVAL '1 day' * retention_days;
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Comment on the function
COMMENT ON FUNCTION prune_old_logs(INTEGER) IS 'Prunes log entries older than specified days (default 7). Returns count of deleted rows.';