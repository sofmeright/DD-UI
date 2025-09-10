BEGIN;

-- Make sure the standard updated_at helper exists
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- If iac_files doesn't exist:
--   - If iac_stack_files exists, rename it.
--   - Else create a fresh iac_files table.
DO $$
BEGIN
  IF to_regclass('public.iac_files') IS NULL THEN
    IF to_regclass('public.iac_stack_files') IS NOT NULL THEN
      EXECUTE 'ALTER TABLE iac_stack_files RENAME TO iac_files';
    ELSE
      EXECUTE $ct$
        CREATE TABLE iac_files (
          id         BIGSERIAL PRIMARY KEY,
          stack_id   BIGINT  NOT NULL REFERENCES iac_stacks(id) ON DELETE CASCADE,
          role       TEXT    NOT NULL CHECK (role IN ('compose','env','script','other')),
          rel_path   TEXT    NOT NULL,
          sops       BOOLEAN NOT NULL DEFAULT FALSE,
          sha256     TEXT    NOT NULL DEFAULT '',
          size       BIGINT  NOT NULL DEFAULT 0,
          created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
          updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
          UNIQUE (stack_id, rel_path)
        )$ct$;
    END IF;
  END IF;

  -- If we came from the old schema, fix column names.
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_name='iac_files' AND column_name='sha256_hex') THEN
    EXECUTE 'ALTER TABLE iac_files RENAME COLUMN sha256_hex TO sha256';
  END IF;
  IF EXISTS (SELECT 1 FROM information_schema.columns
             WHERE table_name='iac_files' AND column_name='size_bytes') THEN
    EXECUTE 'ALTER TABLE iac_files RENAME COLUMN size_bytes TO size';
  END IF;

  -- Ensure defaults / NOT NULL
  EXECUTE 'ALTER TABLE iac_files ALTER COLUMN sha256 SET DEFAULT ''''';
  EXECUTE 'ALTER TABLE iac_files ALTER COLUMN sha256 SET NOT NULL';
  EXECUTE 'ALTER TABLE iac_files ALTER COLUMN size   SET DEFAULT 0';
  EXECUTE 'ALTER TABLE iac_files ALTER COLUMN size   SET NOT NULL';

  -- Ensure UNIQUE (stack_id, rel_path) exists (name may vary, this is just a guard)
  BEGIN
    EXECUTE 'ALTER TABLE iac_files ADD CONSTRAINT iac_files_stack_rel_uniq UNIQUE (stack_id, rel_path)';
  EXCEPTION WHEN duplicate_object THEN
    -- already unique, ignore
  END;
END $$;

-- Helpful indexes
CREATE INDEX IF NOT EXISTS iac_files_stack_idx         ON iac_files(stack_id);
CREATE INDEX IF NOT EXISTS idx_iac_files_stack_role    ON iac_files(stack_id, role);
CREATE INDEX IF NOT EXISTS idx_iac_files_updated_desc  ON iac_files(stack_id, updated_at DESC);

-- Touch updated_at on update
DROP TRIGGER IF EXISTS trg_iac_files_updated ON iac_files;
CREATE TRIGGER trg_iac_files_updated
BEFORE UPDATE ON iac_files
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMIT;