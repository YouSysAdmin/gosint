-- +goose Up
CREATE TABLE sketches (
  id               UUID PRIMARY KEY,
  investigation_id UUID        NOT NULL REFERENCES investigations (id) ON DELETE CASCADE,
  title            TEXT        NOT NULL,
  description      TEXT        NOT NULL DEFAULT '',
  status           TEXT        NOT NULL DEFAULT 'active',
  owner_id         UUID        REFERENCES users (id) ON DELETE SET NULL,
  created_at       TIMESTAMPTZ    NOT NULL,
  updated_at       TIMESTAMPTZ    NOT NULL
);

CREATE INDEX sketches_investigation_id_idx ON sketches (investigation_id);

-- +goose Down
DROP TABLE sketches;
