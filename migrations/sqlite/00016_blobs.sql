-- +goose Up
-- File bytes when files.backend is db, one row per sha256. The fs and
-- s3 backends leave this table empty.
CREATE TABLE blobs (
  hash       TEXT     PRIMARY KEY,
  data       BLOB     NOT NULL,
  size       INTEGER  NOT NULL,
  created_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE blobs;
