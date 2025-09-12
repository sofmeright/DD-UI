-- src/api/migrations/003_hosts_labels_default.sql
BEGIN;

-- create the column if it doesn't exist
ALTER TABLE hosts
  ADD COLUMN IF NOT EXISTS labels jsonb;

-- backfill existing rows
UPDATE hosts
SET labels = '{}'::jsonb
WHERE labels IS NULL;

-- make it NOT NULL with a default going forward
ALTER TABLE hosts
  ALTER COLUMN labels SET DEFAULT '{}'::jsonb,
  ALTER COLUMN labels SET NOT NULL;

COMMIT;