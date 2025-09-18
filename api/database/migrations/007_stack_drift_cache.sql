-- Stack drift cache for hash-based drift detection
CREATE TABLE IF NOT EXISTS stack_drift_cache (
    stack_id BIGINT PRIMARY KEY REFERENCES iac_stacks(id) ON DELETE CASCADE,
    bundle_hash TEXT NOT NULL,
    docker_config_cache JSONB NOT NULL DEFAULT '{}', -- service_name -> config_hash mapping
    last_updated TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_stack_drift_cache_updated ON stack_drift_cache(last_updated);
CREATE INDEX IF NOT EXISTS idx_stack_drift_cache_bundle ON stack_drift_cache(bundle_hash);

-- Comments
COMMENT ON TABLE stack_drift_cache IS 'Stores hash information for efficient drift detection';
COMMENT ON COLUMN stack_drift_cache.bundle_hash IS 'Hash of IaC files (post-SOPS decryption)';
COMMENT ON COLUMN stack_drift_cache.docker_config_cache IS 'Docker config hashes by service name';