-- An entity type an administrator declared as data, beside the
-- built-in ones: fields with no hooks, the primary field the name.
-- Every node reads the table into its type registry on boot and every
-- minute, so a change on one node reaches the others.

-- +goose Up
CREATE TABLE custom_types (
  id          UUID     PRIMARY KEY,
  -- A keyed hash of the name, the name itself in name_sealed.
  name        TEXT     NOT NULL UNIQUE,
  name_sealed TEXT     NOT NULL,
  title       TEXT     NOT NULL,
  description TEXT     NOT NULL DEFAULT '',
  category    TEXT     NOT NULL,
  icon        TEXT     NOT NULL DEFAULT '',
  color       TEXT     NOT NULL DEFAULT '',
  -- JSON list of fields.
  fields      TEXT     NOT NULL,
  created_by  TEXT     NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL,
  updated_at  TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE custom_types;
