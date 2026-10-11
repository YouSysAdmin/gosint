-- +goose Up
CREATE TABLE investigations (
  id          TEXT PRIMARY KEY,
  name        TEXT        NOT NULL,
  description TEXT        NOT NULL DEFAULT '',
  status      TEXT        NOT NULL DEFAULT 'active',
  -- Kept when the owner account goes, the investigation is not theirs alone.
  owner_id    TEXT        REFERENCES users (id) ON DELETE SET NULL,
  created_at  DATETIME    NOT NULL,
  updated_at  DATETIME    NOT NULL
);

-- +goose Down
DROP TABLE investigations;
