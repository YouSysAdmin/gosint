-- A blob no file record names any more, with the moment it was let
-- go. The bytes stay for files.retain_deleted, so an undo brings the
-- file back, then the sweep deletes them unless a record names the
-- hash again. hash is the keyed hash the records hold.

-- +goose Up
CREATE TABLE blob_releases (
  hash        TEXT     PRIMARY KEY,
  released_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE blob_releases;
