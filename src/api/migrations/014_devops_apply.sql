-- src/api/migrations/014_devops_apply.sql
-- Global key/value settings (if not already present)
CREATE TABLE IF NOT EXISTS app_settings (
  key         text PRIMARY KEY,
  value       text NOT NULL,
  updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Per-host Auto DevOps override
CREATE TABLE IF NOT EXISTS host_settings (
  host_name            text PRIMARY KEY,
  auto_apply_override  boolean,
  updated_at           timestamptz NOT NULL DEFAULT now()
);

-- Per-group Auto DevOps override
CREATE TABLE IF NOT EXISTS group_settings (
  group_name           text PRIMARY KEY,
  auto_apply_override  boolean,
  updated_at           timestamptz NOT NULL DEFAULT now()
);

-- Stack-level Auto DevOps override (nullable = inherit)
ALTER TABLE iac_stacks
  ADD COLUMN IF NOT EXISTS auto_apply_override boolean;

-- Ensure iac_enabled exists (used for IaC editor / presence; decoupled from Auto DevOps)
ALTER TABLE iac_stacks
  ADD COLUMN IF NOT EXISTS iac_enabled boolean NOT NULL DEFAULT false;
