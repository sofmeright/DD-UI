-- src/api/migrations/014_devops_apply.sql

-- Global KV if not present
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

-- (Optional) Per-group overrides. Safe to include now for later use.
CREATE TABLE IF NOT EXISTS group_settings (
  group_name           text PRIMARY KEY,
  auto_apply_override  boolean,
  updated_at           timestamptz NOT NULL DEFAULT now()
);

-- Stack-level override column (NULL = inherit)
ALTER TABLE iac_stacks
  ADD COLUMN IF NOT EXISTS auto_apply_override boolean;

-- Keep existing behavior for the editor flag, decoupled from Auto DevOps
ALTER TABLE iac_stacks
  ADD COLUMN IF NOT EXISTS iac_enabled boolean NOT NULL DEFAULT false;