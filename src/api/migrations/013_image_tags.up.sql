-- persist (host,image_id)->(repo,tag), keeping last/first seen
CREATE TABLE IF NOT EXISTS image_tags (
  host_name   TEXT        NOT NULL,
  image_id    TEXT        NOT NULL,  -- full "sha256:..." id
  repo        TEXT        NOT NULL DEFAULT '<none>',
  tag         TEXT        NOT NULL DEFAULT 'none',
  first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (host_name, image_id)
);

CREATE INDEX IF NOT EXISTS image_tags_by_host_repo_tag
  ON image_tags (host_name, repo, tag);