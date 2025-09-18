-- Image tags table (matches current Go usage)
-- Note: deployment_stamps table moved to 003_iac_system.sql after iac_stacks is created
CREATE TABLE IF NOT EXISTS image_tags (
    host_name TEXT NOT NULL,
    image_id TEXT NOT NULL,  -- full "sha256:..." id
    repo TEXT NOT NULL DEFAULT '<none>',
    tag TEXT NOT NULL DEFAULT 'none',
    first_seen TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (host_name, image_id)
);

-- Indexes
CREATE INDEX IF NOT EXISTS image_tags_by_host_repo_tag ON image_tags (host_name, repo, tag);