---
title: Configuration
description: "The gosint.yaml file, GOSINT_* environment variables, and what is set in the console instead."
weight: 10
---

gosint reads its configuration once, at start. There are two places a value
can come from, and a third kind of setting that lives in the database.

## The configuration file

`gosint serve` reads `gosint.yaml` from the current directory. Another file
is named with `--config`:

```bash
gosint serve --config /etc/gosint/gosint.yaml
```

A missing `./gosint.yaml` is not an error: the server starts on its
defaults and the environment. A file named with `--config` that cannot be
read stops the start.

`gosint.example.yaml` in the release archive holds every key with a comment.
Copy it to `gosint.yaml` and change what you need. The defaults suit a
single person on their own machine: the console on `127.0.0.1:4000`, a
SQLite database in `data/gosint.db`.

## Environment variables

Every key can be set as `GOSINT_<SECTION>_<KEY>`, upper case, the dots
turned into underscores:

| Key | Variable |
| --- | --- |
| `database.dsn` | `GOSINT_DATABASE_DSN` |
| `server.public_url` | `GOSINT_SERVER_PUBLIC_URL` |
| `server.tls.mode` | `GOSINT_SERVER_TLS_MODE` |
| `vault.unseal_key_file` | `GOSINT_VAULT_UNSEAL_KEY_FILE` |

An environment variable wins over the file, and the file wins over the
default. A list, such as `server.trusted_proxies`, is given as one comma
separated value: `GOSINT_SERVER_TRUSTED_PROXIES=10.0.0.1, 10.0.0.0/24`.
Durations are written the Go way: `30m`, `168h`.

The container image sets `GOSINT_SERVER_ADDR=0.0.0.0:4000` and
`GOSINT_DATABASE_DSN=/data/gosint.db`, so a container needs no file at all.

## Validation

The configuration is checked before anything opens. A server refuses to
start, and names the key, when for example:

- `database.driver` is neither `sqlite` nor `postgres`, or `database.dsn` is empty
- `server.public_url` is not an `http` or `https` URL with a host
- `server.tls.mode` is on while `server.public_url` says `http://`
- `auth.session_ttl` is under one minute, or `auth.session_max_life` is
  shorter than it (zero is allowed and lifts the cap)
- both `vault.unseal_key` and `vault.unseal_key_file` are given

## Config or setting

The file holds what the process needs before it can read its own database:
where to listen, which database, how to log, how the vault is opened. The
rest is a **setting**, saved by an administrator in the console, sealed in
the database like everything else, and applied without a restart:

| Setting | Console page |
| --- | --- |
| Identity providers for single sign-on | [Identity providers](/administration/sign-in/) |
| The mail relay | [Mail](/administration/mail/) |
| The worker that runs transforms and flows | [Worker](/administration/worker/) |
| Where attached files are stored, their size and retention | [Files](/administration/file-store/) |
| nmap and the private address switches | [Tools](/administration/tools/) |
| The language models of the assistant | [Models](/administration/models/) |
| Entity types of your own | [Entity types](/extending/custom-types/) |

Settings are read when the server is unsealed, on every save, and once a
minute by every node, so a change made on one node of a team install
reaches the others within a minute. No secret of these settings (an SMTP
password, an S3 secret key, a client secret, a model's API key) ever goes
in the YAML file, and none is ever shown again once saved.

## Where to go next

- [Configuration reference](/reference/configuration/) lists every key with
  its type, default and variable.
- [The vault](/administration/vault/) explains the unseal key and the
  `vault` keys.
- [Deployment](/administration/deployment/) covers a reverse proxy, TLS,
  PostgreSQL and Docker.
