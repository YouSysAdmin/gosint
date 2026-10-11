-- The claim on a fresh install: one row at most, written in the same
-- transaction as the first account, so two registrations racing for
-- the administrator seat cannot both win.

-- +goose Up
CREATE TABLE install_claim (
  id INTEGER PRIMARY KEY CHECK (id = 1)
);

-- +goose Down
DROP TABLE install_claim;
