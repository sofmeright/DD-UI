-- 004_runtime_inventory.sql
BEGIN;

CREATE TABLE IF NOT EXISTS stacks (
  id         BIGSERIAL PRIMARY KEY,
  host_id    BIGINT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
  project    TEXT   NOT NULL,
  source     TEXT   NOT NULL DEFAULT 'discovered',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (host_id, project)
);

CREATE TABLE IF NOT EXISTS containers (
  id            BIGSERIAL PRIMARY KEY,
  host_id       BIGINT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
  stack_id      BIGINT     REFERENCES stacks(id) ON DELETE SET NULL,
  container_id  TEXT   NOT NULL,
  name          TEXT   NOT NULL,
  image         TEXT   NOT NULL,
  state         TEXT   NOT NULL,              -- e.g. "running", "exited"
  status        TEXT   NOT NULL,              -- human text from docker
  ports         JSONB  NOT NULL DEFAULT '[]', -- from inspect.NetworkSettings.Ports
  labels        JSONB  NOT NULL DEFAULT '{}', -- Config.Labels
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (host_id, container_id)
);

-- a little housekeeping trigger for updated_at
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_stacks_updated ON stacks;
CREATE TRIGGER trg_stacks_updated BEFORE UPDATE ON stacks
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

DROP TRIGGER IF EXISTS trg_containers_updated ON containers;
CREATE TRIGGER trg_containers_updated BEFORE UPDATE ON containers
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMIT;