ALTER TABLE hosts
  ALTER COLUMN labels SET DEFAULT '{}'::jsonb;

UPDATE hosts SET labels='{}'::jsonb WHERE labels IS NULL;

ALTER TABLE hosts
  ALTER COLUMN labels SET NOT NULL;