-- +goose Up
-- The turns of a chat. Context is the ids of the entities the person
-- had selected when asking, as JSON.
CREATE TABLE chat_messages (
  id         UUID        PRIMARY KEY,
  chat_id    UUID        NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
  role       TEXT        NOT NULL,
  content    TEXT        NOT NULL,
  context    TEXT        NOT NULL DEFAULT '[]',
  created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX chat_messages_chat_id_idx ON chat_messages (chat_id, created_at);

-- +goose Down
DROP TABLE chat_messages;
