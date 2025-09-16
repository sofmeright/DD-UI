-- Docker Cleanup Jobs
-- This migration creates tables to track Docker cleanup operations

CREATE TABLE cleanup_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operation VARCHAR(50) NOT NULL,
    scope VARCHAR(20) NOT NULL CHECK (scope IN ('single_host', 'all_hosts')),
    target VARCHAR(100) NOT NULL,
    status VARCHAR(20) DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'completed', 'failed')),
    dry_run BOOLEAN DEFAULT false,
    force BOOLEAN DEFAULT false,
    exclude_filters JSONB,
    created_at TIMESTAMPTZ DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ DEFAULT now(),
    progress JSONB DEFAULT '{}',
    results JSONB DEFAULT '{}',
    owner VARCHAR(200) NOT NULL
);

-- Indexes for cleanup jobs
CREATE INDEX idx_cleanup_jobs_status ON cleanup_jobs(status);
CREATE INDEX idx_cleanup_jobs_created ON cleanup_jobs(created_at DESC);
CREATE INDEX idx_cleanup_jobs_owner ON cleanup_jobs(owner);
CREATE INDEX idx_cleanup_jobs_operation ON cleanup_jobs(operation);
CREATE INDEX idx_cleanup_jobs_scope_target ON cleanup_jobs(scope, target);

-- Add updated_at trigger
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_cleanup_jobs_updated_at BEFORE UPDATE
    ON cleanup_jobs FOR EACH ROW EXECUTE FUNCTION
    update_updated_at_column();

-- Add constraint to ensure valid operation types
ALTER TABLE cleanup_jobs ADD CONSTRAINT valid_operation
    CHECK (operation IN (
        'system_prune', 
        'image_prune', 
        'container_prune', 
        'volume_prune', 
        'network_prune', 
        'build_cache_prune'
    ));

-- Add constraint to ensure timestamps are logical
ALTER TABLE cleanup_jobs ADD CONSTRAINT logical_timestamps
    CHECK (
        (started_at IS NULL OR started_at >= created_at) AND
        (completed_at IS NULL OR completed_at >= created_at) AND
        (started_at IS NULL OR completed_at IS NULL OR completed_at >= started_at)
    );

-- Add partial index for active jobs
CREATE INDEX idx_cleanup_jobs_active ON cleanup_jobs(created_at DESC) 
    WHERE status IN ('queued', 'running');