-- +goose Up
-- Written analyses of an investigation: notes with references to the
-- entities of its sketches. Content is the editor's document as JSON.
CREATE TABLE analyses (
  id               UUID        PRIMARY KEY,
  investigation_id UUID        NOT NULL REFERENCES investigations (id) ON DELETE CASCADE,
  owner_id         UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  title            TEXT        NOT NULL,
  content          TEXT        NOT NULL DEFAULT '{}',
  created_at       TIMESTAMPTZ NOT NULL,
  updated_at       TIMESTAMPTZ NOT NULL
);

CREATE INDEX analyses_investigation_idx ON analyses (investigation_id, updated_at);

-- +goose Down
DROP TABLE analyses;
