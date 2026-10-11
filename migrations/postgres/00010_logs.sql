-- +goose Up
-- What runs on a sketch said, shown in the console's log panel.
CREATE TABLE logs (
  id         UUID PRIMARY KEY,
  sketch_id  UUID        NOT NULL REFERENCES sketches (id) ON DELETE CASCADE,
  scan_id    UUID        REFERENCES scans (id) ON DELETE SET NULL,
  level      TEXT        NOT NULL,
  message    TEXT        NOT NULL,
  -- Anything structured beside the message, as a JSON object.
  data       TEXT        NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ    NOT NULL
);

CREATE INDEX logs_sketch_id_created_at_idx ON logs (sketch_id, created_at);

-- +goose Down
DROP TABLE logs;
