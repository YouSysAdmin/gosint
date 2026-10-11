-- +goose Up
-- File bytes when files.backend is db, one row per sha256. The fs and
-- s3 backends leave this table empty.
CREATE TABLE blobs (
  hash       TEXT     PRIMARY KEY,
  data       BYTEA     NOT NULL,
  size       BIGINT  NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE blobs;
