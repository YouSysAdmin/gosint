-- +goose Up
CREATE TABLE investigation_members (
  investigation_id UUID        NOT NULL REFERENCES investigations (id) ON DELETE CASCADE,
  user_id          UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  -- owner, admin, editor or viewer. Exactly one owner per investigation.
  role             TEXT        NOT NULL,
  created_at       TIMESTAMPTZ    NOT NULL,
  PRIMARY KEY (investigation_id, user_id)
);

CREATE INDEX investigation_members_user_id_idx ON investigation_members (user_id);

-- +goose Down
DROP TABLE investigation_members;
