-- +goose Up
CREATE TABLE sessions (
  id           TEXT PRIMARY KEY,
  user_id      TEXT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  -- sha256 of the cookie value, the token itself is never stored.
  token_hash   TEXT        NOT NULL UNIQUE,
  user_agent   TEXT        NOT NULL DEFAULT '',
  client_ip    TEXT        NOT NULL DEFAULT '',
  created_at   DATETIME    NOT NULL,
  expires_at   DATETIME    NOT NULL
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- +goose Down
DROP TABLE sessions;
