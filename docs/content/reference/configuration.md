---
title: Configuration file
description: "Every key of gosint.yaml with its type, default and environment variable."
weight: 10
---

`gosint serve` reads `gosint.yaml` from the current directory, or the file
named by `--config`. Every key can also be set in the environment as
`GOSINT_` followed by the key in upper case, dots turned into underscores. The
environment wins over the file, the file over the default. See
[Configuration](/administration/configuration/) for how it fits together.

Types:

- **duration** is written the Go way: `90s`, `30m`, `168h`
- **list** is a YAML list in the file, and one comma separated value in the
  environment: `GOSINT_SERVER_TRUSTED_PROXIES=10.0.0.1, 10.0.1.0/24`
- **bool** is `true` or `false`

Settings made in the console (identity providers, mail, the worker, the file
store, tools, language models) are not in this file. See
[Config or setting](/administration/configuration/#config-or-setting).

## server

| Key | Type | Default | Variable |
| --- | --- | --- | --- |
| `server.addr` | string | `127.0.0.1:4000` | `GOSINT_SERVER_ADDR` |
| `server.public_url` | string | empty | `GOSINT_SERVER_PUBLIC_URL` |
| `server.trusted_proxies` | list | empty | `GOSINT_SERVER_TRUSTED_PROXIES` |

- **`addr`** is the address and port to listen on. Required. The loopback
  default keeps a single-user install off the network. A container or a
  server behind a proxy on another host binds `0.0.0.0:4000`.
- **`public_url`** is the address people reach the console at, when it is not
  the host a request arrives on. Mailed links, the SSO redirect URI and the
  cross-site check are built from it, and an `https` value turns on Secure
  cookies and HSTS. Must be an `http` or `https` URL with a host.
- **`trusted_proxies`** are the addresses or CIDRs of the reverse proxies in
  front. Only for their requests is `X-Forwarded-For` read for the client
  address. See [Deployment](/administration/deployment/#behind-a-reverse-proxy).

### server.tls

| Key | Type | Default | Variable |
| --- | --- | --- | --- |
| `server.tls.mode` | string | `none` | `GOSINT_SERVER_TLS_MODE` |
| `server.tls.cert` | string | empty | `GOSINT_SERVER_TLS_CERT` |
| `server.tls.key` | string | empty | `GOSINT_SERVER_TLS_KEY` |
| `server.tls.watch` | bool | `false` | `GOSINT_SERVER_TLS_WATCH` |
| `server.tls.fqdn` | string | empty | `GOSINT_SERVER_TLS_FQDN` |
| `server.tls.alg` | string | `rsa` | `GOSINT_SERVER_TLS_ALG` |
| `server.tls.cache_dir` | string | empty | `GOSINT_SERVER_TLS_CACHE_DIR` |

- **`mode`** is `none` (plain HTTP, a proxy in front holds the certificate),
  `manual` (serve `cert` and `key`) or `self` (generate a self-signed pair).
  With TLS on, a `public_url` that is set must be `https`.
- **`cert`**, **`key`**: the PEM certificate chain and private key, for
  `manual`.
- **`watch`**: reload `cert` and `key` when the files change, for `manual`.
- **`fqdn`**: the name on a generated certificate, for `self`. Empty takes the
  host of `public_url`, else `localhost`. `localhost`, `127.0.0.1`, `::1` and
  an IP that `addr` binds are always added.
- **`alg`**: `rsa`, `ecdsa` or `ed25519`, for `self`.
- **`cache_dir`**: where the generated pair is kept so a restart serves the
  same one. Empty is a `tls` directory beside the SQLite database, or
  `gosint/tls` in the user configuration directory on PostgreSQL.

## database

| Key | Type | Default | Variable |
| --- | --- | --- | --- |
| `database.driver` | string | `sqlite` | `GOSINT_DATABASE_DRIVER` |
| `database.dsn` | string | `data/gosint.db` | `GOSINT_DATABASE_DSN` |
| `database.max_open_conns` | int | `25` | `GOSINT_DATABASE_MAX_OPEN_CONNS` |
| `database.max_idle_conns` | int | `25` | `GOSINT_DATABASE_MAX_IDLE_CONNS` |
| `database.conn_max_lifetime` | duration | `30m` | `GOSINT_DATABASE_CONN_MAX_LIFETIME` |
| `database.conn_max_idle_time` | duration | `5m` | `GOSINT_DATABASE_CONN_MAX_IDLE_TIME` |

- **`driver`** is `sqlite` (a single file, no server to run) or `postgres`
  (a team install, several nodes).
- **`dsn`** is a file path for SQLite, its directory created if missing, or a
  URL for PostgreSQL: `postgres://gosint:secret@localhost:5432/gosint?sslmode=disable`.
  Required. On SQLite the `files`, `cache`, `transforms` and `tls`
  directories default to beside this file.
- The pool keys bound the connections to the database.

## logging

| Key | Type | Default | Variable |
| --- | --- | --- | --- |
| `logging.level` | string | `info` | `GOSINT_LOGGING_LEVEL` |
| `logging.format` | string | `json` | `GOSINT_LOGGING_FORMAT` |
| `logging.output` | string | `stdout` | `GOSINT_LOGGING_OUTPUT` |
| `logging.color` | bool | `true` | `GOSINT_LOGGING_COLOR` |

- **`level`**: `trace`, `debug`, `info`, `warn` or `error`.
- **`format`**: `json` or `text`.
- **`output`**: `stdout`, `stderr` or a file path.
- **`color`**: colours the `text` format, on an interactive terminal only.

## auth

| Key | Type | Default | Variable |
| --- | --- | --- | --- |
| `auth.session_ttl` | duration | `168h` | `GOSINT_AUTH_SESSION_TTL` |
| `auth.session_max_life` | duration | `720h` | `GOSINT_AUTH_SESSION_MAX_LIFE` |
| `auth.open_registration` | bool | `false` | `GOSINT_AUTH_OPEN_REGISTRATION` |
| `auth.passkeys_enabled` | bool | `true` | `GOSINT_AUTH_PASSKEYS_ENABLED` |
| `auth.rate_limit_per_minute` | int | `10` | `GOSINT_AUTH_RATE_LIMIT_PER_MINUTE` |

- **`session_ttl`**: how long a sign-in lasts. Use renews it once half has
  passed. One minute at least.
- **`session_max_life`**: a sign-in ends this long after it began, however
  often it is used. `0` lifts the cap. Otherwise at least `session_ttl`.
- **`open_registration`**: let anyone reaching the console create an account.
  Off, only the first account (the administrator claiming the install) and
  invited addresses can register.
- **`passkeys_enabled`**: offer passkeys (WebAuthn) beside the password.
- **`rate_limit_per_minute`**: sign-in, registration, unseal and reset
  requests one address may make in a minute. IPv6 addresses count per /64.
  At least 1.

## vault

| Key | Type | Default | Variable |
| --- | --- | --- | --- |
| `vault.unseal_key` | string | empty | `GOSINT_VAULT_UNSEAL_KEY` |
| `vault.unseal_key_file` | string | empty | `GOSINT_VAULT_UNSEAL_KEY_FILE` |
| `vault.master_key` | string | empty | `GOSINT_VAULT_MASTER_KEY` |

Without any of them the server starts sealed and waits for the unseal key in
the console. See [The vault](/administration/vault/).

- **`unseal_key`**: the unseal key itself. It unseals at boot, and initializes
  a server that has no master key yet. Usually given as the variable.
- **`unseal_key_file`**: the same, read from a file such as a mounted secret.
  Give `unseal_key` or `unseal_key_file`, not both.
- **`master_key`**: the raw master key, 32 bytes in base64 (`base64:` prefix
  allowed). For development and tests only: it sits in clear wherever the
  configuration is.

## transforms

| Key | Type | Default | Variable |
| --- | --- | --- | --- |
| `transforms.dir` | string | empty | `GOSINT_TRANSFORMS_DIR` |

- **`dir`**: the directory of your own transform files, templates and
  scripts, read while the server runs. Empty is a `transforms` directory
  beside the SQLite database (created at start), and none on PostgreSQL. See
  [Transform files](/extending/transform-files/).

## console

| Key | Type | Default | Variable |
| --- | --- | --- | --- |
| `console.map_style_light` | string | `https://tiles.openfreemap.org/styles/positron` | `GOSINT_CONSOLE_MAP_STYLE_LIGHT` |
| `console.map_style_dark` | string | `https://tiles.openfreemap.org/styles/dark` | `GOSINT_CONSOLE_MAP_STYLE_DARK` |
| `console.update_check` | bool | `true` | `GOSINT_CONSOLE_UPDATE_CHECK` |

- **`map_style_light`**, **`map_style_dark`**: MapLibre style URLs for the map
  view, loaded by the browser for each theme. Empty draws the points on a
  blank background and the browser reaches no tile server.
- **`update_check`**: ask GitHub for a newer stable release when an
  administrator opens the console, at most twice a day, and show a badge.
  `false` sends nothing. See [Maintenance](/administration/maintenance/#the-release-check).

## The desktop app

`gosint-desktop` reads the same keys from `gosint.yaml` in its own data
directory, and the `GOSINT_*` variables. It changes some defaults: the
database and everything beside it go into the data directory, logs go to
`gosint.log` there as text, and it always listens on the loopback without TLS.
See [Desktop app](/getting-started/desktop-app/).

## A complete example

```yaml
server:
  addr: 127.0.0.1:4000
  public_url: https://osint.example.com
  trusted_proxies: [127.0.0.1]
  tls:
    mode: none

database:
  driver: postgres
  dsn: postgres://gosint:secret@db.internal:5432/gosint?sslmode=require
  max_open_conns: 25
  max_idle_conns: 25
  conn_max_lifetime: 30m
  conn_max_idle_time: 5m

logging:
  level: info
  format: json
  output: stdout
  color: false

auth:
  session_ttl: 168h
  session_max_life: 720h
  open_registration: false
  passkeys_enabled: true
  rate_limit_per_minute: 10

vault:
  unseal_key_file: /run/secrets/unseal_key

transforms:
  dir: /srv/gosint/transforms

console:
  map_style_light: https://tiles.openfreemap.org/styles/positron
  map_style_dark: https://tiles.openfreemap.org/styles/dark
  update_check: true
```
