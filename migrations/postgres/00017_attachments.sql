-- +goose Up
-- Files attached to an entity: a screenshot of a message, a document.
-- The record names the bytes by hash, the bytes live in the blob store.
-- Deleting the node takes its records, the domain then drops blobs no
-- record names.
CREATE TABLE attachments (
  id           UUID     PRIMARY KEY,
  sketch_id    UUID     NOT NULL REFERENCES sketches (id) ON DELETE CASCADE,
  node_id      UUID     NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
  name         TEXT     NOT NULL,
  content_type TEXT     NOT NULL,
  size         BIGINT  NOT NULL,
  -- A keyed hash of the content's sha256, the name the sealed bytes
  -- are stored under, the sha256 itself in digest_sealed.
  hash         TEXT     NOT NULL,
  digest_sealed TEXT     NOT NULL,
  uploader_id  UUID     NOT NULL,
  -- What the file says about itself (EXIF, a document's properties),
  -- JSON, empty text until read.
  metadata     TEXT     NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX attachments_node_idx ON attachments (node_id, created_at);
CREATE INDEX attachments_hash_idx ON attachments (hash);

-- +goose Down
DROP TABLE attachments;
