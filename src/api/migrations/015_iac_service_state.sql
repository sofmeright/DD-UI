-- src/api/migrations/015_iac_service_state.sql
BEGIN;

CREATE TABLE IF NOT EXISTS iac_service_state (
  id                BIGSERIAL PRIMARY KEY,
  stack_id          BIGINT NOT NULL REFERENCES iac_stacks(id) ON DELETE CASCADE,
  service_name      TEXT   NOT NULL,
  last_deploy_uid   TEXT   NOT NULL,  -- stable until spec digest changes
  last_spec_digest  TEXT   NOT NULL,  -- deterministic spec hash
  enrolled          BOOLEAN NOT NULL DEFAULT TRUE,
  first_seen        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (stack_id, service_name)
);

CREATE INDEX IF NOT EXISTS iac_service_state_stack_idx ON iac_service_state(stack_id);

CREATE OR REPLACE FUNCTION set_updated_at_iac_service_state()
RETURNS TRIGGER AS $$
BEGIN NEW.updated_at = now(); RETURN NEW; END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_iac_service_state_updated ON iac_service_state;
CREATE TRIGGER trg_iac_service_state_updated
BEFORE UPDATE ON iac_service_state
FOR EACH ROW EXECUTE FUNCTION set_updated_at_iac_service_state();

COMMIT;
