-- src/api/migrations/005_add_owner.sql
BEGIN;

-- HOSTS
ALTER TABLE hosts
  ADD COLUMN IF NOT EXISTS owner TEXT;
UPDATE hosts
  SET owner = COALESCE(NULLIF(owner, ''), 'unassigned')
WHERE owner IS NULL OR owner = '';
ALTER TABLE hosts
  ALTER COLUMN owner SET DEFAULT 'unassigned',
  ALTER COLUMN owner SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_hosts_owner ON hosts(owner);

-- STACKS
ALTER TABLE stacks
  ADD COLUMN IF NOT EXISTS owner TEXT;
UPDATE stacks
  SET owner = COALESCE(NULLIF(owner, ''), 'unassigned')
WHERE owner IS NULL OR owner = '';
ALTER TABLE stacks
  ALTER COLUMN owner SET DEFAULT 'unassigned',
  ALTER COLUMN owner SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_stacks_owner ON stacks(owner);

-- CONTAINERS
ALTER TABLE containers
  ADD COLUMN IF NOT EXISTS owner TEXT;
UPDATE containers
  SET owner = COALESCE(NULLIF(owner, ''), 'unassigned')
WHERE owner IS NULL OR owner = '';
ALTER TABLE containers
  ALTER COLUMN owner SET DEFAULT 'unassigned',
  ALTER COLUMN owner SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_containers_owner ON containers(owner);

COMMIT;