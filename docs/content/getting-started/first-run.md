---
title: First run
description: "Start the server, choose the unseal key that protects everything it stores, and create the administrator."
weight: 20
---

A fresh install needs two things from you before it does anything useful:
an unseal key, which encrypts everything the server stores, and an
administrator account. Both are set in the browser on the first visit.

## Start the server

```bash
gosint serve
```

Without a configuration file the server listens on `127.0.0.1:4000` and keeps
its SQLite database in `data/gosint.db`, relative to the directory you started
it from. Open <http://127.0.0.1:4000/> in a browser.

To use a configuration file, copy `gosint.example.yaml` from the release
archive to `gosint.yaml` in the same directory, or point at one anywhere with
`--config`:

```bash
gosint serve --config /etc/gosint/gosint.yaml
```

## Choose the unseal key

The server starts **sealed**. Everything it stores is encrypted under a master
key, and that key is kept only in a wrapped form that your unseal key opens.
Until it is given, every page sends you to the unseal page and no transform
runs.

On a fresh install the page reads **Protect this server**. Type an unseal key
of at least 12 characters, type it again under **Unseal key again**, and press
**Encrypt and unseal**.

{{< callout type="danger" title="A lost unseal key cannot be recovered" >}}
Nobody can read the data without the unseal key, not even with full access to
the database. There is no reset and no back door. Keep it in a password
manager or a safe place before you press **Encrypt and unseal**.
{{< /callout >}}

After every restart the page reads **Unseal this server**. Enter the same
unseal key and press **Unseal**.

On a server that has to start unattended, the unseal key can be read at boot
from a file (`vault.unseal_key_file`) or from the `GOSINT_VAULT_UNSEAL_KEY`
variable. The data is then only as safe as that file. You can also initialize
from the command line with `gosint vault init`, and change the unseal key later
with `gosint vault rekey`. See [The vault](/administration/vault/).

## Create the administrator

With the server unsealed, a fresh install has no account yet, so the sign-in
page sends you to **Set up gosint**. Enter an email address and a password (8
characters or more) twice, then press **Create the administrator**.

The first account is always the administrator. After it, registration is
closed: other people join by an invitation from an investigation or by an
account an administrator creates. See [Accounts](/administration/accounts/).

## A minimal configuration

Most installs need only a few keys. Every key can also be set as an
environment variable, `GOSINT_<SECTION>_<KEY>`, for example
`GOSINT_DATABASE_DSN`.

```yaml
server:
  # Bind wider when a reverse proxy in front reaches this host.
  addr: 127.0.0.1:4000
  # Where people reach the console, used in links sent by mail.
  public_url: https://osint.example.com

database:
  driver: sqlite
  dsn: /var/lib/gosint/gosint.db
```

Mail, sign-in providers, language models, the worker and the file store are
not in the file. An administrator sets them in the console, where they are
stored encrypted with the rest. The full list of keys is in the
[configuration reference](/reference/configuration/).

## Where the data lives

With SQLite everything sits beside the database file, `data/` by default:

| Path | What it holds |
| --- | --- |
| `data/gosint.db` | The database, encrypted values inside |
| `data/files/` | Files attached to entities, encrypted |
| `data/cache/` | Reference data transforms download, such as CDN ranges |
| `data/transforms/` | Your own [transform files](/extending/transform-files/) |
| `data/tls/` | The generated certificate, when the server does TLS itself |

Back up the database and the files directory together. A backup is useless
without the unseal key, and so is the unseal key without a backup.

For a team install on PostgreSQL behind a reverse proxy, see
[Deployment](/administration/deployment/).

## Next

[Your first investigation](/getting-started/first-investigation/): make a
sketch, add an entity and run a transform on it.
