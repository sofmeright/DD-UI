-- Migration 015: Add deployment stamps for drift detection
-- This migration adds tracking for deployment fingerprints to enable
-- reliable drift detection even with environment variable resolution

-- Table to track deployment events with fingerprints
CREATE TABLE IF NOT EXISTS deployment_stamps (
    id BIGSERIAL PRIMARY KEY,
    stack_id BIGINT NOT NULL REFERENCES iac_stacks(id) ON DELETE CASCADE,
    deployment_hash TEXT NOT NULL, -- SHA256 of normalized deployment config
    deployment_timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    deployment_method TEXT NOT NULL DEFAULT 'compose', -- compose|script|manual
    deployment_user TEXT,
    deployment_env_hash TEXT, -- Hash of resolved environment variables
    deployment_status TEXT NOT NULL DEFAULT 'pending', -- pending|success|failed
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Index for efficient lookups
CREATE INDEX IF NOT EXISTS idx_deployment_stamps_stack_id ON deployment_stamps(stack_id);
CREATE INDEX IF NOT EXISTS idx_deployment_stamps_hash ON deployment_stamps(deployment_hash);
CREATE INDEX IF NOT EXISTS idx_deployment_stamps_timestamp ON deployment_stamps(deployment_timestamp DESC);

-- Add deployment stamp reference to containers
ALTER TABLE containers ADD COLUMN IF NOT EXISTS deployment_stamp_id BIGINT REFERENCES deployment_stamps(id);
ALTER TABLE containers ADD COLUMN IF NOT EXISTS deployment_hash TEXT;

-- Index for container deployment tracking
CREATE INDEX IF NOT EXISTS idx_containers_deployment_stamp_id ON containers(deployment_stamp_id);
CREATE INDEX IF NOT EXISTS idx_containers_deployment_hash ON containers(deployment_hash);

-- Add trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_deployment_stamps_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_deployment_stamps_updated_at
    BEFORE UPDATE ON deployment_stamps
    FOR EACH ROW
    EXECUTE FUNCTION update_deployment_stamps_updated_at();