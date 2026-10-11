-- Identity providers an administrator configures at runtime, so a
-- team signs in through its own OIDC. Every setting but the switches
-- is sealed by the vault.

-- +goose Up
CREATE TABLE oauth_providers (
  id                     UUID PRIMARY KEY,
  name                   TEXT NOT NULL,
  -- A keyed hash of the slug the sign-in route names, the slug itself
  -- in slug_sealed.
  slug                   TEXT NOT NULL UNIQUE,
  slug_sealed            TEXT NOT NULL,
  type                   TEXT NOT NULL DEFAULT 'oidc',
  client_id              TEXT NOT NULL DEFAULT '',
  client_secret          TEXT NOT NULL DEFAULT '',
  issuer                 TEXT NOT NULL DEFAULT '',
  auth_url               TEXT NOT NULL DEFAULT '',
  token_url              TEXT NOT NULL DEFAULT '',
  userinfo_url           TEXT NOT NULL DEFAULT '',
  scopes                 TEXT NOT NULL DEFAULT '[]',
  enabled                BOOLEAN NOT NULL DEFAULT TRUE,
  hidden                 BOOLEAN NOT NULL DEFAULT FALSE,
  auto_register          BOOLEAN NOT NULL DEFAULT TRUE,
  require_email_verified BOOLEAN NOT NULL DEFAULT TRUE,
  -- An existing account is linked by a verified address only when set.
  link_by_email          BOOLEAN NOT NULL DEFAULT FALSE,
  -- Discovery and token calls may reach a private address, an IdP
  -- inside the network.
  allow_private_targets  BOOLEAN NOT NULL DEFAULT FALSE,
  allowed_domains        TEXT NOT NULL DEFAULT '[]',
  allowed_emails         TEXT NOT NULL DEFAULT '[]',
  groups_claim           TEXT NOT NULL DEFAULT '',
  allowed_groups         TEXT NOT NULL DEFAULT '[]',
  created_at             TIMESTAMPTZ NOT NULL,
  updated_at             TIMESTAMPTZ NOT NULL
);

CREATE INDEX oauth_providers_login_idx ON oauth_providers (enabled, hidden);

-- +goose Down
DROP TABLE oauth_providers;
