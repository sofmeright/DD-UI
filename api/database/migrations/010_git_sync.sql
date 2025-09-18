-- Git repository synchronization for full DD-UI configuration
-- This syncs the entire /data directory structure with inventory and compose files
CREATE TABLE IF NOT EXISTS git_sync_config (
    id SERIAL PRIMARY KEY,
    repo_url TEXT NOT NULL,
    branch TEXT DEFAULT 'main',
    auth_token TEXT, -- GitHub/GitLab token (encrypted if SOPS available)
    ssh_key TEXT, -- Alternative SSH key for git operations
    commit_author_name TEXT DEFAULT 'DD-UI',
    commit_author_email TEXT DEFAULT 'ddui@localhost',
    sync_enabled BOOLEAN DEFAULT FALSE,
    sync_mode TEXT DEFAULT 'off', -- 'off', 'push', 'pull', 'sync'
    force_on_conflict BOOLEAN DEFAULT FALSE, -- Force overwrites on conflicts
    auto_push BOOLEAN DEFAULT FALSE, -- DEPRECATED: use sync_mode
    auto_pull BOOLEAN DEFAULT FALSE, -- DEPRECATED: use sync_mode
    pull_interval_minutes INT DEFAULT 5,
    push_on_change BOOLEAN DEFAULT TRUE, -- DEPRECATED: use sync_mode
    sync_path TEXT DEFAULT '/data', -- Local path to sync
    last_sync_hash TEXT, -- Track last sync commit for conflict detection
    last_pull_at TIMESTAMP WITH TIME ZONE,
    last_push_at TIMESTAMP WITH TIME ZONE,
    last_commit_hash TEXT,
    last_sync_status TEXT,
    last_sync_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Git sync operations log
CREATE TABLE IF NOT EXISTS git_sync_log (
    id BIGSERIAL PRIMARY KEY,
    operation TEXT NOT NULL, -- 'clone', 'pull', 'push', 'commit', 'merge'
    status TEXT NOT NULL, -- 'success', 'failed', 'conflict', 'in_progress'
    commit_before TEXT,
    commit_after TEXT,
    files_changed TEXT[], -- Array of changed file paths
    additions INT DEFAULT 0,
    deletions INT DEFAULT 0,
    message TEXT,
    error_details TEXT,
    initiated_by TEXT NOT NULL, -- User email or 'system' for auto-sync
    duration_ms INT,
    started_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

-- Track individual file conflicts for manual resolution
CREATE TABLE IF NOT EXISTS git_sync_conflicts (
    id BIGSERIAL PRIMARY KEY,
    file_path TEXT NOT NULL,
    conflict_type TEXT NOT NULL, -- 'merge', 'permission', 'deletion'
    local_content TEXT,
    remote_content TEXT,
    resolved BOOLEAN DEFAULT FALSE,
    resolution_type TEXT, -- 'local', 'remote', 'manual'
    resolved_by TEXT,
    resolved_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_git_sync_log_operation ON git_sync_log(operation);
CREATE INDEX idx_git_sync_log_status ON git_sync_log(status);
CREATE INDEX idx_git_sync_log_started_at ON git_sync_log(started_at DESC);
CREATE INDEX idx_git_sync_conflicts_resolved ON git_sync_conflicts(resolved);
CREATE INDEX idx_git_sync_conflicts_file_path ON git_sync_conflicts(file_path);

-- Triggers
CREATE TRIGGER git_sync_config_updated_at 
    BEFORE UPDATE ON git_sync_config 
    FOR EACH ROW 
    EXECUTE FUNCTION set_updated_at();

-- Initialize with empty config (only if table is empty)
INSERT INTO git_sync_config (
    repo_url,
    branch,
    sync_enabled,
    sync_mode,
    force_on_conflict,
    commit_author_name,
    commit_author_email
) 
SELECT '', 'main', FALSE, 'off', FALSE, 'DD-UI Bot', 'ddui@localhost'
WHERE NOT EXISTS (SELECT 1 FROM git_sync_config);

-- Add columns only if table exists and columns don't exist
-- This handles upgrades from older versions
DO $$ 
BEGIN
    -- Only try to add columns if the table exists
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'git_sync_config') THEN
        BEGIN
            ALTER TABLE git_sync_config ADD COLUMN sync_mode TEXT DEFAULT 'off';
        EXCEPTION
            WHEN duplicate_column THEN NULL;
        END;
        
        BEGIN
            ALTER TABLE git_sync_config ADD COLUMN force_on_conflict BOOLEAN DEFAULT FALSE;
        EXCEPTION
            WHEN duplicate_column THEN NULL;
        END;
        
        BEGIN
            ALTER TABLE git_sync_config ADD COLUMN last_sync_hash TEXT;
        EXCEPTION
            WHEN duplicate_column THEN NULL;
        END;
    END IF;
END $$;