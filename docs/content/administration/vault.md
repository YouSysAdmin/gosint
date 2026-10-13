---
title: The vault and the unseal key
description: "Why the server starts sealed, how to initialize and unseal it, change the unseal key, and seal it again."
weight: 20
---

Everything gosint stores is encrypted under one **master key**: the graph,
the files, accounts, secrets, flows, chats, logs. The master key is never
written to disk in clear. It is kept in the database **wrapped** under an
**unseal key** that you choose, and the server holds the master key in
memory only.

{{< callout type="danger" title="The unseal key cannot be recovered" >}}
There is no reset and no back door. Lose the unseal key and nobody can read
the data again, not you, not an administrator, not the project. Keep it in a
password manager or another safe place, away from the server and its backups.
{{< /callout >}}

## The sealed start

After every start the server is **sealed**: it knows where the data is but
cannot open any of it. While sealed:

- every API call but the seal endpoints answers `503 sealed`, and the
  console sends you to the unseal page
- the worker claims no scan and the scheduled jobs are skipped
- nothing a transform or a flow needs can be read

Entering the unseal key opens the master key on that node, and everything
starts.

## Initializing a new server

A fresh install has no master key yet. The first time you open the console
it shows **Protect this server**: choose an unseal key of **12 characters
or more**, type it twice, and press **Encrypt and unseal**. The server makes
a master key, wraps it under your unseal key, and opens.

The same can be done from the command line before the first start, which
also suits a server you set up over SSH:

```bash
gosint vault init
# New unseal key:
# Again:
```

`gosint vault init` reads the same configuration as `gosint serve` (use
`--config` for another file) and applies the database migrations first. It
refuses a server that is initialized already.

The first initialization is first come, first served, like the first account.
Open the console of a new install before you expose it, or initialize it
with the CLI.

## Unsealing after a restart

### In the console

Open the console: it shows **Unseal this server**. Enter the unseal key and
press **Unseal**. A wrong key is refused no sooner than 750 ms, and the
attempts count against the same per-address limit as sign-ins.

### Unattended

A server that must come back on its own after a reboot can be given the
unseal key in its configuration:

- `vault.unseal_key_file`: a file holding the key, for example a Docker or
  Kubernetes secret mounted at `/run/secrets/unseal_key`
- `GOSINT_VAULT_UNSEAL_KEY`: the key itself, in the environment

With either, the server unseals at boot, and initializes a server that has no
key yet with it. Give one or the other, not both.

{{< callout type="warning" title="Unattended means the key sits beside the data" >}}
The data is then only as safe as that file or that environment. Anyone who
can read both the database and the key file can read everything. The server
logs a warning on every such start.
{{< /callout >}}

## Changing the unseal key

`gosint vault rekey` asks for the current unseal key and a new one twice.
The master key does not change, only its wrapping, so no data is re-encrypted
and the change is instant.

```bash
gosint vault rekey
# Current unseal key:
# New unseal key:
# Again:
```

Every node reads the same wrapped key from the database, so a rekey applies
to the whole install. A running node keeps working: it already holds the
master key. The next unseal of any node takes the new unseal key.

Both commands read the keys without echo from a terminal, or line by line
from a pipe.

## Sealing a node

**Admin > Maintenance > Seal this server** (Admin is in the top bar of an administrator) forgets the master key
on the node answering the request. Running scans fail, and nothing opens
until someone enters the unseal key again. Use it when you suspect the
machine and want it closed without stopping the process.

## Several nodes

On PostgreSQL every node of the install shares one wrapped key, and **each
node is unsealed on its own**: entering the key on one node does not open
another. Unseal each node through its own address, or give all of them
`vault.unseal_key_file`. See [Deployment](/administration/deployment/).

## vault.master_key, for development only

`vault.master_key` is the raw master key itself, 32 bytes in base64 (a
`base64:` prefix is allowed). It unseals at boot and sits in clear wherever
the configuration is, so it is meant for development and tests only. When
the database already holds a wrapped key, the configured one must be the
same key or the server refuses to start.

`gosint vault init` with `vault.master_key` set wraps that key under a new
unseal key. Remove `master_key` from the configuration afterwards and the
server starts sealed like any other.

## What the unseal key does not protect

- An export downloaded **without** a passphrase is plain JSON. Exports are
  sealed under their own passphrase, not under the vault, because they move
  to other installs.
- A machine where the server runs unsealed has the master key in memory.
- See [Security model](/administration/security/) for what stays in clear
  in the database.
