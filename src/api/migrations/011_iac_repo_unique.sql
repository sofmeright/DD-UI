BEGIN;

-- Deduplicate rows that would violate the new unique constraint (keep the lowest id)
DELETE FROM iac_repos a
USING iac_repos b
WHERE a.id > b.id
  AND a.kind = b.kind
  AND COALESCE(a.root_path, '') = COALESCE(b.root_path, '');

-- Add the unique constraint that the upsert will target
ALTER TABLE iac_repos
  ADD CONSTRAINT iac_repos_kind_root_uniq UNIQUE (kind, root_path);

COMMIT;