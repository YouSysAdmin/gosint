-- Recovery codes, the way back in when the authenticator is gone. Ten
-- single-use codes minted when the second factor is turned on, shown
-- once and stored as sha256, spent by a conditional update so two
-- nodes cannot both honour one.

-- +goose Up
CREATE TABLE user_recovery_codes (
  id         UUID PRIMARY KEY,
  user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  code_hash  TEXT NOT NULL,
  used_at    TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  UNIQUE (user_id, code_hash)
);

CREATE INDEX user_recovery_codes_user_idx ON user_recovery_codes (user_id);

-- +goose Down
DROP TABLE user_recovery_codes;
