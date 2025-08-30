-- Backfill any existing NULL owners
UPDATE hosts SET owner = '' WHERE owner IS NULL;

-- Ensure a default and not-null going forward
ALTER TABLE hosts
  ALTER COLUMN owner SET DEFAULT '',
  ALTER COLUMN owner SET NOT NULL;