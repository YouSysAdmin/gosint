-- The search index over sealed labels: a keyed hash of every two and
-- three character piece of a node's lowercased label, so a piece of a
-- label is found in SQL without the label in clear.

-- +goose Up
CREATE TABLE node_grams (
  node_id   UUID NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
  sketch_id UUID NOT NULL,
  gram      TEXT NOT NULL,
  PRIMARY KEY (node_id, gram)
);

CREATE INDEX node_grams_gram ON node_grams (gram, node_id);

-- +goose Down
DROP TABLE node_grams;
