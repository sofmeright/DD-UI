BEGIN;

CREATE TABLE IF NOT EXISTS scan_logs (
  id       BIGSERIAL PRIMARY KEY,
  host_id  BIGINT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
  at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  level    TEXT NOT NULL DEFAULT 'info',
  message  TEXT NOT NULL,
  data     JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_scan_logs_host_at ON scan_logs(host_id, at DESC);

COMMIT;