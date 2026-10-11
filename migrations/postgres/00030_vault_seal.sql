-- The master key, wrapped under the operator's passphrase. One row at
-- most. The server starts sealed and unwraps it when the passphrase is
-- given, so the database alone opens nothing.

-- +goose Up
CREATE TABLE vault_seal (
  id         INTEGER PRIMARY KEY CHECK (id = 1),
  -- The envelope: Argon2id parameters, salt, nonce and the sealed key.
  wrapped    TEXT    NOT NULL,
  -- A known text sealed under the master key, to tell that a key given
  -- in the config is the one the data is sealed under.
  key_check  TEXT    NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE vault_seal;
