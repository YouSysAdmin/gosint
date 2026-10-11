-- +goose Up
CREATE TABLE edges (
  id         TEXT PRIMARY KEY,
  sketch_id  TEXT        NOT NULL REFERENCES sketches (id) ON DELETE CASCADE,
  source_id  TEXT        NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
  target_id  TEXT        NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
  -- A keyed hash of the relation, the relation itself in label_sealed.
  label      TEXT        NOT NULL,
  label_sealed TEXT        NOT NULL,
  properties TEXT        NOT NULL DEFAULT '{}',
  created_at DATETIME    NOT NULL,
  updated_at DATETIME    NOT NULL,
  UNIQUE (sketch_id, source_id, target_id, label)
);

-- The unique key serves lookups by source, this one serves neighbors by target.
CREATE INDEX edges_target_id_idx ON edges (target_id);

-- +goose Down
DROP TABLE edges;
