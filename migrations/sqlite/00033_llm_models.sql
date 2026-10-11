-- The language models an administrator offers the assistant, one
-- picked by each person. Everything but the default switch is sealed
-- JSON, the server's API key included.

-- +goose Up
CREATE TABLE llm_models (
  id         TEXT PRIMARY KEY,
  data       TEXT NOT NULL,
  is_default BOOLEAN NOT NULL DEFAULT FALSE,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE llm_models;
