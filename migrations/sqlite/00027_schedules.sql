-- A flow run again on a sketch every so often, by the maintenance
-- jobs, on a fixed selection or on every entity of the flow's input
-- type at the time. The last two scans it queued let the console say
-- what the run found and what the one before saw that this did not.

-- +goose Up
CREATE TABLE schedules (
  id            TEXT     PRIMARY KEY,
  sketch_id     TEXT     NOT NULL REFERENCES sketches (id) ON DELETE CASCADE,
  owner_id      TEXT     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  flow_id       TEXT     NOT NULL,
  flow_name     TEXT     NOT NULL,
  -- JSON list of node ids, empty for every entity of the input type.
  node_ids      TEXT     NOT NULL DEFAULT '[]',
  every_seconds INTEGER  NOT NULL,
  enabled       BOOLEAN  NOT NULL DEFAULT TRUE,
  -- Bumped by each run, the claim a node makes before queueing.
  version       INTEGER  NOT NULL DEFAULT 0,
  last_run_at   DATETIME,
  last_scan_id  TEXT     NOT NULL DEFAULT '',
  prev_scan_id  TEXT     NOT NULL DEFAULT '',
  last_error    TEXT     NOT NULL DEFAULT '',
  created_at    DATETIME NOT NULL,
  updated_at    DATETIME NOT NULL
);

CREATE INDEX schedules_sketch_idx ON schedules (sketch_id, created_at);

-- +goose Down
DROP TABLE schedules;
