-- A hand edit of a sketch's graph, kept so it can be undone: what the
-- touched nodes and edges were before and after. Per account, so one
-- person undoes their own edits in order.

-- +goose Up
CREATE TABLE graph_changes (
  id           TEXT PRIMARY KEY,
  sketch_id    TEXT NOT NULL REFERENCES sketches (id) ON DELETE CASCADE,
  user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  -- What it was, as the Undo button says it.
  kind         TEXT NOT NULL,
  -- JSON snapshots of the touched nodes and edges, secrets sealed.
  before_state TEXT NOT NULL,
  after_state  TEXT NOT NULL,
  undone       BOOLEAN NOT NULL DEFAULT FALSE,
  created_at   DATETIME NOT NULL
);

CREATE INDEX graph_changes_stack_idx ON graph_changes (sketch_id, user_id, id);

-- +goose Down
DROP TABLE graph_changes;
