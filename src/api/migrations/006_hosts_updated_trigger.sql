-- src/api/migrations/006_hosts_updated_trigger.sql
BEGIN;

-- (Re)define helper in case someone skipped 004 (harmless if it already exists)
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_hosts_updated ON hosts;
CREATE TRIGGER trg_hosts_updated
BEFORE UPDATE ON hosts
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMIT;