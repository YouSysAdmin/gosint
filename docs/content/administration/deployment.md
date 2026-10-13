---
title: Deployment
description: "Behind a reverse proxy, TLS on the listener, PostgreSQL with several nodes, Docker, and backups."
weight: 110
---

gosint is one binary holding the API, the console and the worker. A single
person runs it on their own machine with the defaults. This page is about a
server other people reach.

## Behind a reverse proxy

The usual setup: gosint listens on the loopback, a reverse proxy (Caddy,
nginx, Traefik) holds the certificate and forwards to it.

```yaml
server:
  addr: 127.0.0.1:4000
  public_url: https://osint.example.com
  trusted_proxies: [127.0.0.1]
```

- **`server.public_url`** is the address people type. Mailed links and the
  SSO redirect URI are built from it, and the cross-site check compares
  requests against it. When it is `https`, cookies are marked Secure and the
  server sends HSTS, even though the proxy talks to gosint over plain HTTP.
- **`server.trusted_proxies`** lists the proxies, as addresses or CIDRs.
  Only for a request from one of them is `X-Forwarded-For` read, walked from
  the right past the listed proxies, so the sign-in limiter sees the real
  client and a client cannot choose its own address. Without it every client
  shares the proxy's address and its rate limit.

The proxy must pass streaming responses through unbuffered: the live log
(`/api/sketches/<id>/events`) and the assistant's answers are Server-Sent
Events. In nginx, `proxy_buffering off` on those locations, or for the whole
site. Raise the proxy's body size limit to the largest file you accept.

## TLS on the listener

When nothing sits in front, gosint can do the handshake itself with
`server.tls.mode`:

| Mode | What it does |
| --- | --- |
| `none` | Plain HTTP, the default |
| `manual` | Serves `cert` and `key`, a PEM pair you provide and renew. With `watch: true` a renewed pair is picked up without a restart |
| `self` | Generates a self-signed pair for `fqdn` (else the host of `public_url`, else `localhost`), with `localhost`, `127.0.0.1` and `::1` always on it. `alg` is `rsa`, `ecdsa` or `ed25519`. The pair is kept in `cache_dir` (a `tls` directory beside the SQLite database by default) so a restart serves the same one. Browsers warn about it |

```yaml
server:
  addr: 0.0.0.0:443
  public_url: https://osint.example.com
  tls:
    mode: manual
    cert: /etc/gosint/tls/fullchain.pem
    key: /etc/gosint/tls/privkey.pem
    watch: true
```

With TLS on, a `public_url` that is set must be `https`, or the server refuses
to start. There is no ACME client built in: use a proxy or a certificate tool
for automatic certificates.

## PostgreSQL and several nodes

SQLite suits one person or a small team on one machine. PostgreSQL lets
several gosint processes (nodes) serve the same install.

```yaml
database:
  driver: postgres
  dsn: postgres://gosint:secret@db.internal:5432/gosint?sslmode=require
  max_open_conns: 25
```

What changes with several nodes:

- **Every node runs everything**: the API, the console, a worker and the
  scheduled jobs. Put them behind one load balancer, no sticky sessions
  needed.
- **Every node is unsealed on its own.** Unseal each through its own address,
  or give all of them `vault.unseal_key_file`. See [The vault](/administration/vault/).
- **Live events cross nodes** through PostgreSQL `LISTEN/NOTIFY`, so a log
  line written by a worker on one node reaches a browser connected to another.
- **The file store must be shared.** The default on PostgreSQL is the
  database itself. A directory must be one every node mounts, or use S3. See
  [File store](/administration/file-store/).
- **Migrations** run at start under a PostgreSQL advisory lock, so nodes
  starting together do not race. `gosint serve --skip-migrate` only checks
  that the schema is current and refuses to start otherwise, for a rollout
  where one node migrates and the others wait.
- **Transform files** (`transforms.dir`) have no default on PostgreSQL. Set a
  directory, and give every node the same files.
- **Tools** such as nmap must be installed on every node.

Settings saved in the console reach the other nodes within a minute.

## Docker

The image `ghcr.io/yousysadmin/gosint` holds the binary, nmap, and the
interpreters a transform script is likely to need (bash, Python with common
OSINT libraries, curl, jq). It runs as an unprivileged user, listens on port
4000 and keeps everything under the `/data` volume.

```yaml
services:
  gosint:
    image: ghcr.io/yousysadmin/gosint:latest
    restart: unless-stopped
    init: true
    environment:
      GOSINT_SERVER_PUBLIC_URL: https://osint.example.com
      GOSINT_SERVER_TRUSTED_PROXIES: 172.16.0.0/12
      # GOSINT_VAULT_UNSEAL_KEY_FILE: /run/secrets/unseal_key
    ports:
      - "127.0.0.1:4000:4000"
    volumes:
      - gosint-data:/data
    stop_grace_period: 30s

volumes:
  gosint-data:
```

The repository's `compose.yaml` is a fuller version with a read-only root
filesystem, dropped capabilities and a health check on `/health`. Pin a
release tag in production, `latest` moves with every release. Give the
container time to stop (`stop_grace_period`) so its worker hands running
scans back to the queue.

## Backups

Back up three things, and keep the third apart from the first two:

1. **The database.** The SQLite file (with its `-wal` file, or use
   `sqlite3 gosint.db ".backup backup.db"` on a running server), or a
   `pg_dump` of PostgreSQL.
2. **The file store**, unless it is the database: the `files` directory or
   the bucket.
3. **The unseal key**, somewhere else entirely: a password manager, a safe.

Both the database and the file store are encrypted, so a backup on its own
reveals nothing. That also means a backup without the unseal key is useless.
A restore needs the database and the files from the same moment, and the
unseal key that was current then (a later `vault rekey` changes the key the
wrapped master key needs).

The cache directory and the generated TLS pair need no backup.
