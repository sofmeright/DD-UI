-- src/api/migrations/012_add_iac_indexes.sql
-- Speed up per-host/group IaC lookups
CREATE INDEX IF NOT EXISTS idx_iac_stacks_scope
  ON iac_stacks (scope_kind, scope_name);

-- Fast file listing and role filters (CANONICAL TABLE)
CREATE INDEX IF NOT EXISTS idx_iac_files_stack_role
  ON iac_files (stack_id, role);

-- Optional: recently updated per stack
CREATE INDEX IF NOT EXISTS idx_iac_files_stack_updated
  ON iac_files (stack_id, updated_at DESC);
