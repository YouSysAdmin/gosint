-- Invitations to an investigation by link. The clear token leaves once,
-- in the link, only its hash is stored. A pending invitation lets the
-- invited address register while registration is closed.

-- +goose Up
CREATE TABLE investigation_invitations (
  id               TEXT PRIMARY KEY,
  investigation_id TEXT NOT NULL REFERENCES investigations (id) ON DELETE CASCADE,
  email            TEXT NOT NULL,
  role             TEXT NOT NULL,
  token_hash       TEXT NOT NULL UNIQUE,
  -- Who invited, kept as an id with no constraint so the row outlives
  -- the inviter.
  invited_by       TEXT NOT NULL DEFAULT '',
  created_at       DATETIME NOT NULL,
  expires_at       DATETIME NOT NULL,
  accepted_at      DATETIME
);

CREATE INDEX investigation_invitations_inv_idx ON investigation_invitations (investigation_id, created_at);

-- +goose Down
DROP TABLE investigation_invitations;
