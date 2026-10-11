-- +goose Up
-- One run of a transform or a flow. The table is also the job queue:
-- a worker claims a pending row and holds it by renewing lease_until.
CREATE TABLE scans (
  id           UUID PRIMARY KEY,
  sketch_id    UUID        REFERENCES sketches (id) ON DELETE CASCADE,
  owner_id     UUID        REFERENCES users (id) ON DELETE SET NULL,
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
  lease_until  TIMESTAMPTZ   ,
  created_at   TIMESTAMPTZ    NOT NULL,
  started_at   TIMESTAMPTZ   ,
  completed_at TIMESTAMPTZ   
);

CREATE INDEX scans_status_created_at_idx ON scans (status, created_at);
CREATE INDEX scans_sketch_id_idx ON scans (sketch_id, created_at);

-- +goose Down
DROP TABLE scans;
