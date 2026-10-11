-- An external identity linked to an account. A subject is unique only
-- within its issuer, so the key is the pair, and one person may hold
-- an identity at several providers.

-- +goose Up
CREATE TABLE oauth_identities (
  id            TEXT PRIMARY KEY,
  user_id       TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  provider_id   TEXT NOT NULL REFERENCES oauth_providers (id) ON DELETE CASCADE,
  -- A keyed hash of the subject, the subject itself in subject_sealed.
  subject       TEXT NOT NULL,
  subject_sealed TEXT NOT NULL,
  email         TEXT NOT NULL DEFAULT '',
  name          TEXT NOT NULL DEFAULT '',
  created_at    DATETIME NOT NULL,
  last_login_at DATETIME NOT NULL,
  UNIQUE (provider_id, subject),
  UNIQUE (user_id, provider_id)
);

CREATE INDEX oauth_identities_user_idx ON oauth_identities (user_id);

-- +goose Down
DROP TABLE oauth_identities;
