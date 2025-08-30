-- src/api/migrations/002_hosts_groups_default.sql
-- Default [] for text[] column
ALTER TABLE hosts
  ALTER COLUMN "groups" SET DEFAULT ARRAY[]::text[];

-- Backfill any existing NULLs
UPDATE hosts
SET "groups" = ARRAY[]::text[]
WHERE "groups" IS NULL;

-- Keep it NOT NULL (no-op if already set)
ALTER TABLE hosts
  ALTER COLUMN "groups" SET NOT NULL;