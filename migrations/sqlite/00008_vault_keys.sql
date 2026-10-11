-- +goose Up
CREATE TABLE vault_keys (
  id          TEXT PRIMARY KEY,
  owner_id    TEXT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  -- A keyed hash of what transforms ask for, such as WHOXY_API_KEY,
  -- the name itself in name_sealed: the names say which services a
  -- person pays for.
  name        TEXT        NOT NULL,
  name_sealed TEXT        NOT NULL,
  -- The secret, sealed under its owner.
  value       TEXT        NOT NULL,
  created_at  DATETIME    NOT NULL,
  updated_at  DATETIME    NOT NULL,
  UNIQUE (owner_id, name)
);

-- +goose Down
DROP TABLE vault_keys;
