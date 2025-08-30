BEGIN;

ALTER TABLE hosts
  ADD COLUMN IF NOT EXISTS owner TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_hosts_owner ON hosts(owner);

ALTER TABLE stacks
  ADD COLUMN IF NOT EXISTS owner TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_stacks_owner ON stacks(owner);

ALTER TABLE containers
  ADD COLUMN IF NOT EXISTS owner TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_containers_owner ON containers(owner);

COMMIT;