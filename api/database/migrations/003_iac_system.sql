-- Infrastructure as Code (IaC) system tables
CREATE TYPE iac_source_kind AS ENUM ('local','git');
CREATE TYPE iac_scope_kind  AS ENUM ('host','group');
CREATE TYPE iac_deploy_kind AS ENUM ('compose','script','unmanaged');
CREATE TYPE iac_sops_status AS ENUM ('all','partial','none');

-- Drop existing tables if they exist to ensure clean schema
DROP TABLE IF EXISTS iac_deployments CASCADE;
DROP TABLE IF EXISTS iac_services CASCADE;  
DROP TABLE IF EXISTS iac_stack_files CASCADE;
DROP TABLE IF EXISTS iac_stacks CASCADE;
DROP TABLE IF EXISTS iac_repos CASCADE;

CREATE TABLE iac_repos (
    id BIGSERIAL PRIMARY KEY,
    kind iac_source_kind NOT NULL DEFAULT 'local',
    root_path TEXT,
    url TEXT,
    branch TEXT,
    last_commit TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_scan_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(kind, root_path)
);

CREATE TABLE iac_stacks (
    id BIGSERIAL PRIMARY KEY,
    repo_id BIGINT NOT NULL REFERENCES iac_repos(id) ON DELETE CASCADE,
    scope_kind iac_scope_kind NOT NULL,
    scope_name TEXT NOT NULL,
    stack_name TEXT NOT NULL,
    rel_path TEXT NOT NULL,
    compose_file TEXT,
    deploy_kind iac_deploy_kind NOT NULL DEFAULT 'unmanaged',
    pull_policy TEXT DEFAULT 'missing',
    sops_status iac_sops_status NOT NULL DEFAULT 'none',
    auto_apply BOOLEAN DEFAULT FALSE,
    allow_apply BOOLEAN DEFAULT TRUE,
    auto_apply_override BOOLEAN,
    auto_devops_override TEXT, -- Added for auto_devops_policy.go
    iac_enabled BOOLEAN DEFAULT FALSE,
    last_scan_at TIMESTAMP WITH TIME ZONE,
    compose TEXT DEFAULT '',
    env TEXT DEFAULT '',
    hash VARCHAR(64) DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(repo_id, scope_kind, scope_name, stack_name)
);

CREATE TABLE iac_services (
    id BIGSERIAL PRIMARY KEY,
    stack_id BIGINT NOT NULL REFERENCES iac_stacks(id) ON DELETE CASCADE,
    service_name TEXT NOT NULL,
    container_name TEXT,
    image TEXT,
    labels JSONB DEFAULT '{}',
    env_keys JSONB DEFAULT '[]',
    env_files JSONB DEFAULT '[]',
    ports JSONB DEFAULT '[]',
    volumes JSONB DEFAULT '[]',
    deploy JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(stack_id, service_name)
);

CREATE TABLE iac_stack_files (
    id BIGSERIAL PRIMARY KEY,
    stack_id BIGINT NOT NULL REFERENCES iac_stacks(id) ON DELETE CASCADE,
    role TEXT DEFAULT 'unknown',
    rel_path TEXT DEFAULT '',
    sops BOOLEAN DEFAULT FALSE,
    sha256_hex TEXT DEFAULT '',
    size_bytes BIGINT DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(stack_id, rel_path)
);

CREATE TABLE iac_deployments (
    id BIGSERIAL PRIMARY KEY,
    stack_id BIGINT NOT NULL REFERENCES iac_stacks(id),
    hosts TEXT[] NOT NULL,
    output TEXT DEFAULT '',
    success BOOLEAN DEFAULT FALSE,
    started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

-- Deployment stamps table (moved from 002_runtime_inventory.sql to resolve dependency order)
CREATE TABLE IF NOT EXISTS deployment_stamps (
    id BIGSERIAL PRIMARY KEY,
    host_id BIGINT REFERENCES hosts(id) ON DELETE CASCADE,
    stack_id BIGINT NOT NULL REFERENCES iac_stacks(id) ON DELETE CASCADE,
    deployment_hash TEXT NOT NULL,
    deployment_timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deployment_method TEXT NOT NULL DEFAULT 'compose',
    deployment_user TEXT,
    deployment_env_hash TEXT,
    deployment_status TEXT NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(stack_id, deployment_hash)
);

-- Override system tables (for auto_devops_policy.go)
CREATE TABLE IF NOT EXISTS iac_overrides (
    id BIGSERIAL PRIMARY KEY,
    level TEXT NOT NULL, -- 'host', 'group', 'global'
    scope_name TEXT, -- host_name or group_name (null for global)
    stack_name TEXT, -- null for global settings
    key TEXT, -- for global key-value settings
    value TEXT, -- for global key-value settings
    auto_devops_override TEXT, -- 'enable', 'disable', etc.
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS settings (
    id BIGSERIAL PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    value TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_iac_stacks_stack_name ON iac_stacks(stack_name);
CREATE INDEX IF NOT EXISTS idx_iac_stacks_hash ON iac_stacks(hash);
CREATE INDEX IF NOT EXISTS idx_iac_services_stack_id ON iac_services(stack_id);
CREATE INDEX IF NOT EXISTS idx_iac_stack_files_stack_id ON iac_stack_files(stack_id);
CREATE INDEX IF NOT EXISTS idx_iac_deployments_stack_id ON iac_deployments(stack_id);
CREATE INDEX IF NOT EXISTS idx_deployment_stamps_host_id ON deployment_stamps(host_id);
CREATE INDEX IF NOT EXISTS idx_deployment_stamps_stack_id ON deployment_stamps(stack_id);
CREATE INDEX IF NOT EXISTS idx_deployment_stamps_hash ON deployment_stamps(deployment_hash);
CREATE INDEX IF NOT EXISTS idx_deployment_stamps_status ON deployment_stamps(deployment_status);
CREATE INDEX IF NOT EXISTS idx_deployment_stamps_timestamp ON deployment_stamps(deployment_timestamp);
CREATE INDEX IF NOT EXISTS idx_iac_overrides_level_scope ON iac_overrides(level, scope_name);
CREATE INDEX IF NOT EXISTS idx_iac_overrides_stack_name ON iac_overrides(stack_name);
CREATE INDEX IF NOT EXISTS idx_iac_overrides_key ON iac_overrides(key);
CREATE INDEX IF NOT EXISTS idx_settings_key ON settings(key);

-- Triggers
CREATE TRIGGER iac_repos_updated_at 
    BEFORE UPDATE ON iac_repos 
    FOR EACH ROW 
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER iac_stacks_updated_at 
    BEFORE UPDATE ON iac_stacks 
    FOR EACH ROW 
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER iac_services_updated_at 
    BEFORE UPDATE ON iac_services 
    FOR EACH ROW 
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER iac_stack_files_updated_at 
    BEFORE UPDATE ON iac_stack_files 
    FOR EACH ROW 
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER deployment_stamps_updated_at 
    BEFORE UPDATE ON deployment_stamps 
    FOR EACH ROW 
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER iac_overrides_updated_at 
    BEFORE UPDATE ON iac_overrides 
    FOR EACH ROW 
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER settings_updated_at 
    BEFORE UPDATE ON settings 
    FOR EACH ROW 
    EXECUTE FUNCTION set_updated_at();

-- Add foreign key constraint to containers table for deployment_stamp_id
ALTER TABLE containers 
    ADD CONSTRAINT fk_containers_deployment_stamp 
    FOREIGN KEY (deployment_stamp_id) 
    REFERENCES deployment_stamps(id) 
    ON DELETE SET NULL;