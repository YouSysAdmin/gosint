-- +goose Up
CREATE TABLE investigations (
  id          UUID PRIMARY KEY,
  name        TEXT        NOT NULL,
  description TEXT        NOT NULL DEFAULT '',
  status      TEXT        NOT NULL DEFAULT 'active',
  -- Kept when the owner account goes, the investigation is not theirs alone.
  owner_id    UUID        REFERENCES users (id) ON DELETE SET NULL,
  created_at  TIMESTAMPTZ    NOT NULL,
  updated_at  TIMESTAMPTZ    NOT NULL
);

-- +goose Down
DROP TABLE investigations;
