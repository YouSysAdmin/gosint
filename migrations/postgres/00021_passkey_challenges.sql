-- Spent WebAuthn challenges. A ceremony rides in a sealed cookie, and
-- a copy taken before it was cleared would answer again on any node,
-- so a finished ceremony records its challenge here and a second
-- finish is refused. Rows matter until the ceremony would have expired
-- and are pruned on every claim.

-- +goose Up
CREATE TABLE passkey_challenges (
  challenge  TEXT PRIMARY KEY,
  expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX passkey_challenges_expires_idx ON passkey_challenges (expires_at);

-- +goose Down
DROP TABLE passkey_challenges;
