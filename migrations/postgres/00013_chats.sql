-- +goose Up
-- Conversations with the assistant about a sketch, each an account's own.
CREATE TABLE chats (
  id         UUID        PRIMARY KEY,
  sketch_id  UUID        NOT NULL REFERENCES sketches (id) ON DELETE CASCADE,
  owner_id   UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  title      TEXT        NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX chats_sketch_owner_idx ON chats (sketch_id, owner_id);

-- +goose Down
DROP TABLE chats;
