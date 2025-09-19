-- Groups table for managing host groups
CREATE TABLE IF NOT EXISTS groups (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    tags TEXT[] DEFAULT '{}',
    parent_id BIGINT REFERENCES groups(id) ON DELETE CASCADE,
    vars JSONB DEFAULT '{}',
    owner TEXT NOT NULL DEFAULT 'unassigned',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for groups
CREATE INDEX IF NOT EXISTS idx_groups_parent_id ON groups(parent_id);
CREATE INDEX IF NOT EXISTS idx_groups_tags ON groups USING GIN(tags);
CREATE INDEX IF NOT EXISTS idx_groups_owner ON groups(owner);
CREATE INDEX IF NOT EXISTS idx_groups_name ON groups(name);

-- Host-Group relationships
CREATE TABLE IF NOT EXISTS host_groups (
    id BIGSERIAL PRIMARY KEY,
    host_id BIGINT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    direct_member BOOLEAN DEFAULT TRUE,
    inherited_from BIGINT REFERENCES groups(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(host_id, group_id)
);

-- Indexes for host_groups
CREATE INDEX IF NOT EXISTS idx_host_groups_host_id ON host_groups(host_id);
CREATE INDEX IF NOT EXISTS idx_host_groups_group_id ON host_groups(group_id);

-- Add metadata columns to hosts for DD-UI inventory management
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS tags TEXT[] DEFAULT '{}';
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS alt_name TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS tenant TEXT;
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS allowed_users TEXT[];
ALTER TABLE hosts ADD COLUMN IF NOT EXISTS env JSONB DEFAULT '{}';

-- Add metadata columns to groups for DD-UI inventory management
ALTER TABLE groups ADD COLUMN IF NOT EXISTS alt_name TEXT;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS tenant TEXT;
ALTER TABLE groups ADD COLUMN IF NOT EXISTS allowed_users TEXT[];
ALTER TABLE groups ADD COLUMN IF NOT EXISTS env JSONB DEFAULT '{}';

-- Create indexes for metadata columns
CREATE INDEX IF NOT EXISTS idx_hosts_tags ON hosts USING GIN(tags);
CREATE INDEX IF NOT EXISTS idx_hosts_tenant ON hosts(tenant);
CREATE INDEX IF NOT EXISTS idx_hosts_alt_name ON hosts(alt_name);
CREATE INDEX IF NOT EXISTS idx_groups_tenant ON groups(tenant);
CREATE INDEX IF NOT EXISTS idx_groups_alt_name ON groups(alt_name);

-- Trigger to update groups.updated_at
CREATE OR REPLACE FUNCTION update_groups_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER groups_updated_at_trigger
BEFORE UPDATE ON groups
FOR EACH ROW
EXECUTE FUNCTION update_groups_updated_at();