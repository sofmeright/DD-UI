BEGIN;

CREATE TYPE iac_source_kind AS ENUM ('local','git');
CREATE TYPE iac_scope_kind  AS ENUM ('host','group');
CREATE TYPE iac_deploy_kind AS ENUM ('compose','script','unmanaged');
CREATE TYPE iac_sops_status AS ENUM ('all','partial','none');

CREATE TABLE IF NOT EXISTS iac_repos (
  id            BIGSERIAL PRIMARY KEY,
  kind          iac_source_kind NOT NULL DEFAULT 'local',
  root_path     TEXT,                -- for local
  url           TEXT,                -- for git
  branch        TEXT,
  last_commit   TEXT,
  enabled       BOOLEAN NOT NULL DEFAULT TRUE,
  last_scan_at  TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS iac_stacks (
  id            BIGSERIAL PRIMARY KEY,
  repo_id       BIGINT NOT NULL REFERENCES iac_repos(id) ON DELETE CASCADE,
  scope_kind    iac_scope_kind NOT NULL,       -- host|group
  scope_name    TEXT NOT NULL,                 -- host name OR group name
  stack_name    TEXT NOT NULL,
  rel_path      TEXT NOT NULL,                 -- path from repo root (docker-compose/…)
  compose_file  TEXT,                          -- relative to rel_path
  deploy_kind   iac_deploy_kind NOT NULL DEFAULT 'unmanaged',
  pull_policy   TEXT,
  sops_status   iac_sops_status NOT NULL DEFAULT 'none',
  iac_enabled   BOOLEAN NOT NULL DEFAULT TRUE,
  last_commit   TEXT,
  last_scan_at  TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (repo_id, scope_kind, scope_name, stack_name)
);

CREATE TABLE IF NOT EXISTS iac_services (
  id              BIGSERIAL PRIMARY KEY,
  stack_id        BIGINT NOT NULL REFERENCES iac_stacks(id) ON DELETE CASCADE,
  service_name    TEXT NOT NULL,
  container_name  TEXT,
  image           TEXT,
  labels          JSONB NOT NULL DEFAULT '{}',
  env_keys        JSONB NOT NULL DEFAULT '[]',     -- ["FOO","BAR"]
  env_files       JSONB NOT NULL DEFAULT '[]',     -- [{path:"...",sops:true}]
  ports           JSONB NOT NULL DEFAULT '[]',     -- normalized published/target/proto
  volumes         JSONB NOT NULL DEFAULT '[]',     -- normalized volumes
  deploy          JSONB NOT NULL DEFAULT '{}',     -- restart/update bits
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (stack_id, service_name)
);

CREATE TABLE IF NOT EXISTS iac_stack_files (
  id            BIGSERIAL PRIMARY KEY,
  stack_id      BIGINT NOT NULL REFERENCES iac_stacks(id) ON DELETE CASCADE,
  role          TEXT NOT NULL,                   -- compose|env|script|other
  rel_path      TEXT NOT NULL,
  sops          BOOLEAN NOT NULL DEFAULT FALSE,
  sha256_hex    TEXT,
  size_bytes    BIGINT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (stack_id, rel_path)
);

-- record of actual deployments (used later for script drift)
CREATE TABLE IF NOT EXISTS iac_deployments (
  id            BIGSERIAL PRIMARY KEY,
  stack_id      BIGINT NOT NULL REFERENCES iac_stacks(id) ON DELETE CASCADE,
  method        iac_deploy_kind NOT NULL,
  deployed_at   TIMESTAMPTZ NOT NULL,
  actor         TEXT,
  revision_sha  TEXT,
  notes         TEXT
);

-- updated_at triggers
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$ BEGIN NEW.updated_at = now(); RETURN NEW; END; $$ LANGUAGE plpgsql;

CREATE TRIGGER trg_iac_repos_updated     BEFORE UPDATE ON iac_repos     FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_iac_stacks_updated    BEFORE UPDATE ON iac_stacks    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_iac_services_updated  BEFORE UPDATE ON iac_services  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_iac_files_updated     BEFORE UPDATE ON iac_stack_files FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMIT;
