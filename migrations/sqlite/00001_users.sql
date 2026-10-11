-- Accounts. The first account is the administrator, account_type is 1
-- for a local password account and 2 for one that signs in through an
-- identity provider. Every text column is sealed by the vault (gs1.).

-- +goose Up
CREATE TABLE users (
  id                 TEXT     PRIMARY KEY,
  -- A keyed hash of the lowercased address, which the sign-in and the
  -- unique key match, the address itself in email_sealed.
  email              TEXT     NOT NULL UNIQUE,
  email_sealed       TEXT     NOT NULL,
  password_hash      TEXT     NOT NULL,
  first_name         TEXT     NOT NULL DEFAULT '',
  last_name          TEXT     NOT NULL DEFAULT '',
  avatar_url         TEXT     NOT NULL DEFAULT '',
  is_active          BOOLEAN  NOT NULL DEFAULT TRUE,
  admin              BOOLEAN  NOT NULL DEFAULT FALSE,
  account_type       INTEGER  NOT NULL DEFAULT 1,
  -- The authenticator secret, '' for none.
  totp_secret        TEXT     NOT NULL DEFAULT '',
  totp_enabled       BOOLEAN  NOT NULL DEFAULT FALSE,
  -- The last time step a code was accepted for, so a code works once.
  totp_last_step     INTEGER  NOT NULL DEFAULT 0,
  totp_failures      INTEGER  NOT NULL DEFAULT 0,
  totp_locked_until  DATETIME,
  login_failures     INTEGER  NOT NULL DEFAULT 0,
  login_locked_until DATETIME,
  created_at         DATETIME NOT NULL,
  updated_at         DATETIME NOT NULL
);

-- +goose Down
DROP TABLE users;
