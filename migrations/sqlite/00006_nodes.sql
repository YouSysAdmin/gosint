-- The entities of a sketch. Every text column but the drawing style
-- (colour, icon, flag, shape) is sealed by the vault.

-- +goose Up
CREATE TABLE nodes (
  id         TEXT PRIMARY KEY,
  sketch_id  TEXT             NOT NULL REFERENCES sketches (id) ON DELETE CASCADE,
  -- A keyed hash of the entity type, the type itself in type_sealed.
  node_type  TEXT             NOT NULL,
  type_sealed TEXT             NOT NULL,
  -- A keyed hash of the type and the entity's identity within it, its
  -- primary field such as the domain name, the key itself in
  -- key_sealed. The label is display only and may change as properties
  -- are learned, the key does not.
  node_key   TEXT             NOT NULL,
  key_sealed TEXT             NOT NULL,
  node_label TEXT             NOT NULL,
  -- JSON object, the entity fields. Merged key by key on upsert.
  properties TEXT             NOT NULL DEFAULT '{}',
  metadata   TEXT             NOT NULL DEFAULT '{}',
  color      TEXT             NOT NULL DEFAULT '',
  icon       TEXT             NOT NULL DEFAULT '',
  image      TEXT             NOT NULL DEFAULT '',
  flag       TEXT             NOT NULL DEFAULT '',
  shape      TEXT             NOT NULL DEFAULT '',
  size       INTEGER,
  x          REAL,
  y          REAL,
  -- Bumped on every write, the compare-and-swap for property merges.
  version    INTEGER          NOT NULL DEFAULT 1,
  created_at DATETIME         NOT NULL,
  updated_at DATETIME         NOT NULL,
  -- An entity is one node per sketch, so a transform finding it again
  -- lands on the same node.
  UNIQUE (sketch_id, node_type, node_key)
);

-- +goose Down
DROP TABLE nodes;
