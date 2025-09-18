-- Core schema with hosts, common functions, and base tables
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Hosts table
CREATE TABLE IF NOT EXISTS hosts (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    addr VARCHAR(255) NOT NULL,
    vars JSONB NOT NULL DEFAULT '{}',
    "groups" TEXT[] NOT NULL DEFAULT '{}', -- quoted because groups is reserved
    labels JSONB NOT NULL DEFAULT '{}',
    owner VARCHAR(255) DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Stacks table (runtime inventory)
CREATE TABLE IF NOT EXISTS stacks (
    id BIGSERIAL PRIMARY KEY,
    host_id BIGINT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    project VARCHAR(255) NOT NULL,
    source VARCHAR(255) DEFAULT 'unknown',
    owner VARCHAR(255) DEFAULT '',
    auto_apply_override BOOLEAN DEFAULT NULL,
    iac_enabled BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(host_id, project)
);

-- Containers table (runtime inventory)
CREATE TABLE IF NOT EXISTS containers (
    id BIGSERIAL PRIMARY KEY,
    host_id BIGINT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    stack_id BIGINT REFERENCES stacks(id) ON DELETE CASCADE,
    container_id VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    image VARCHAR(255) NOT NULL,
    state VARCHAR(50) DEFAULT 'unknown',
    status VARCHAR(255) DEFAULT '',
    ports JSONB NOT NULL DEFAULT '[]',
    labels JSONB NOT NULL DEFAULT '{}',
    owner VARCHAR(255) DEFAULT '',
    created_ts TIMESTAMP WITH TIME ZONE,
    ip_addr VARCHAR(45) DEFAULT '',
    env JSONB NOT NULL DEFAULT '[]',
    networks JSONB NOT NULL DEFAULT '[]',
    mounts JSONB NOT NULL DEFAULT '[]',
    deployment_stamp_id BIGINT,
    deployment_hash VARCHAR(255) DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(host_id, container_id)
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_hosts_name ON hosts(name);
CREATE INDEX IF NOT EXISTS idx_hosts_owner ON hosts(owner);
CREATE INDEX IF NOT EXISTS idx_stacks_host_id ON stacks(host_id);
CREATE INDEX IF NOT EXISTS idx_stacks_project ON stacks(project);
CREATE INDEX IF NOT EXISTS idx_stacks_owner ON stacks(owner);
CREATE INDEX IF NOT EXISTS idx_containers_host_id ON containers(host_id);
CREATE INDEX IF NOT EXISTS idx_containers_stack_id ON containers(stack_id);
CREATE INDEX IF NOT EXISTS idx_containers_name ON containers(name);
CREATE INDEX IF NOT EXISTS idx_containers_owner ON containers(owner);

-- Triggers
CREATE TRIGGER hosts_updated_at 
    BEFORE UPDATE ON hosts 
    FOR EACH ROW 
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER stacks_updated_at 
    BEFORE UPDATE ON stacks 
    FOR EACH ROW 
    EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER containers_updated_at 
    BEFORE UPDATE ON containers 
    FOR EACH ROW 
    EXECUTE FUNCTION set_updated_at();