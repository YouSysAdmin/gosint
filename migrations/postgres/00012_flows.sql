-- +goose Up
-- Chains of transforms (core/flow), each an account's own.
CREATE TABLE flows (
  id          UUID PRIMARY KEY,
  owner_id    UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  -- A keyed hash of the name, the name itself in name_sealed.
  name        TEXT        NOT NULL,
  name_sealed TEXT        NOT NULL,
  description TEXT        NOT NULL DEFAULT '',
  -- A keyed hash of the entity type the flow starts from.
  input_type  TEXT        NOT NULL,
  -- flow.Definition as JSON: the input type, the steps, the editor layout.
  definition  TEXT        NOT NULL,
  created_at  TIMESTAMPTZ    NOT NULL,
  updated_at  TIMESTAMPTZ    NOT NULL,
  UNIQUE (owner_id, name)
);

-- +goose Down
DROP TABLE flows;
