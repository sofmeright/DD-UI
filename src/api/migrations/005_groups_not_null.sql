-- Ensure groups has a default and no NULLs
ALTER TABLE hosts
  ALTER COLUMN "groups" SET DEFAULT '{}'::text[];

UPDATE hosts
  SET "groups" = '{}'::text[]
  WHERE "groups" IS NULL;

ALTER TABLE hosts
  ALTER COLUMN "groups" SET NOT NULL;