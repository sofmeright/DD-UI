-- src/api/migrations/001_init.sql
-- hosts discovered/imported from inventory
CREATE TABLE IF NOT EXISTS hosts (
  id          bigserial PRIMARY KEY,
  name        text NOT NULL UNIQUE,
  addr        text,
  vars        jsonb NOT NULL DEFAULT '{}'::jsonb,
  groups      text[] NOT NULL DEFAULT '{}',
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_hosts_name ON hosts(name);
