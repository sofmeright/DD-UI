-- src/api/migrations/009_containers_created_ip.sql
BEGIN;

ALTER TABLE containers
  ADD COLUMN IF NOT EXISTS created_ts TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS ip_addr     TEXT,
  ADD COLUMN IF NOT EXISTS env         JSONB NOT NULL DEFAULT '[]',  -- keep original docker []"KEY=VAL"
  ADD COLUMN IF NOT EXISTS networks    JSONB NOT NULL DEFAULT '{}',  -- inspect.NetworkSettings.Networks
  ADD COLUMN IF NOT EXISTS mounts      JSONB NOT NULL DEFAULT '[]';  -- inspect.Mounts

COMMIT;