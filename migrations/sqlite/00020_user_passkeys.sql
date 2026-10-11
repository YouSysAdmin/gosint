-- Passkeys (WebAuthn) on accounts, a row per authenticator: a
-- discoverable sign-in carries no email, the credential id alone
-- names the owner. The account id is the WebAuthn user handle.

-- +goose Up
CREATE TABLE user_passkeys (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  -- The authenticator's raw credential id, base64url. Unique across
  -- the install, an assertion names this and nothing else.
  credential_id TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  -- The go-webauthn credential as JSON: public key, transports and the
  -- sign counter. No secret, the private half never leaves the
  -- authenticator.
  credential    TEXT NOT NULL,
  created_at    DATETIME NOT NULL,
  last_used_at  DATETIME
);

CREATE INDEX user_passkeys_user_idx ON user_passkeys (user_id);

-- +goose Down
DROP TABLE user_passkeys;
