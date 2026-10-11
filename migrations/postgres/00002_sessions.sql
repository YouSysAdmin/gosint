-- +goose Up
CREATE TABLE sessions (
  id           UUID PRIMARY KEY,
  user_id      UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  -- sha256 of the cookie value, the token itself is never stored.
  token_hash   TEXT        NOT NULL UNIQUE,
  user_agent   TEXT        NOT NULL DEFAULT '',
  client_ip    TEXT        NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ    NOT NULL,
  expires_at   TIMESTAMPTZ    NOT NULL
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- +goose Down
DROP TABLE sessions;
