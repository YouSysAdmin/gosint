-- +goose Up
-- Transforms declared as YAML (core/template). The YAML is the source of
-- truth, the other columns are read from it on save for listing.
CREATE TABLE transform_templates (
  id          UUID PRIMARY KEY,
  owner_id    UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  -- A keyed hash of the name, the name itself in name_sealed.
  name        TEXT        NOT NULL,
  name_sealed TEXT        NOT NULL,
  description TEXT        NOT NULL DEFAULT '',
  category    TEXT        NOT NULL,
  input_type  TEXT        NOT NULL,
  output_type TEXT        NOT NULL,
  source      TEXT        NOT NULL,
  -- Listed and runnable by every account. One that reads secrets
  -- runs for its owner only, see domain/transformtemplate.
  is_public   BOOLEAN     NOT NULL DEFAULT FALSE,
  created_at  TIMESTAMPTZ    NOT NULL,
  updated_at  TIMESTAMPTZ    NOT NULL,
  UNIQUE (owner_id, name)
);

-- +goose Down
DROP TABLE transform_templates;
