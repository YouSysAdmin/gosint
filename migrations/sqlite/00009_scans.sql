-- +goose Up
-- One run of a transform or a flow. The table is also the job queue:
-- a worker claims a pending row and holds it by renewing lease_until.
CREATE TABLE scans (
  id           TEXT PRIMARY KEY,
  sketch_id    TEXT        REFERENCES sketches (id) ON DELETE CASCADE,
  owner_id     TEXT        REFERENCES users (id) ON DELETE SET NULL,
  -- What runs it, such as transform or flow, and which one.
  kind         TEXT        NOT NULL,
  name         TEXT        NOT NULL,
  -- pending, running, completed, failed or cancelled.
  status       TEXT        NOT NULL,
  input        TEXT        NOT NULL DEFAULT '{}',
  details      TEXT        NOT NULL DEFAULT '{}',
  error        TEXT        NOT NULL DEFAULT '',
  attempts     INTEGER     NOT NULL DEFAULT 0,
  claimed_by   TEXT        NOT NULL DEFAULT '',
  lease_until  DATETIME   ,
  created_at   DATETIME    NOT NULL,
  started_at   DATETIME   ,
  completed_at DATETIME   
);

CREATE INDEX scans_status_created_at_idx ON scans (status, created_at);
CREATE INDEX scans_sketch_id_idx ON scans (sketch_id, created_at);

-- +goose Down
DROP TABLE scans;
