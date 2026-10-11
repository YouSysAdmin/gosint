-- Install-wide settings an administrator edits in the console, one row
-- per name (mail), the value sealed JSON, secrets included.

-- +goose Up
CREATE TABLE settings (
  name       TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_by TEXT NOT NULL DEFAULT '',
  updated_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE settings;
