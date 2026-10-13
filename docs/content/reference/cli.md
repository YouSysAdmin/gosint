---
title: Command line
description: "Every command and flag of the gosint binary and of gosint-desktop."
weight: 20
---

Most of gosint is run from the console. The command line starts the server
and handles the unseal key.

## gosint

```text
gosint [command] [--config <file>]
```

| Flag | Meaning |
| --- | --- |
| `--config <file>` | The configuration file. Every command takes it. Default `./gosint.yaml`, and a missing default file is not an error |
| `-h`, `--help` | Help for any command |
| `-v`, `--version` | The version, the same line as `gosint version` |

Run without a command, `gosint` prints the help.

### gosint serve

```bash
gosint serve [--skip-migrate]
```

Runs the API, the console and the worker until it receives `SIGINT` or
`SIGTERM`. It migrates the database to the current schema first, then starts
sealed unless the configuration gives an unseal key (see
[The vault](/administration/vault/)). On a stop it finishes requests in flight
for up to 15 seconds and hands running scans back to the queue.

| Flag | Meaning |
| --- | --- |
| `--skip-migrate` | Do not apply migrations: check the schema is current and refuse to start otherwise. For a rollout where one node migrates and the others wait |

The console is served under `/app`, the API under `/api`, and `/health` answers
for a load balancer.

### gosint vault init

```bash
gosint vault init
```

Wraps the master key under a new unseal key, asked twice, 12 characters at
least. Use it before the first start of a server instead of choosing the key
in the console. When `vault.master_key` is set (development), that key is the
one wrapped. Refuses a server that is initialized already, and a database that
holds sealed data but no wrapped key.

### gosint vault rekey

```bash
gosint vault rekey
```

Changes the unseal key: asks the current one, then the new one twice. The
master key and the data stay as they are, so the change is instant and applies
to every node at its next unseal.

Both `vault` commands read the configuration like `serve`, migrate the
database to the current schema, and read the keys without echo from a
terminal, or one per line from standard input when it is a pipe:

```bash
printf '%s\n%s\n%s\n' "$OLD" "$NEW" "$NEW" | gosint vault rekey
```

### gosint version

```bash
gosint version
# gosint 1.4.0
```

`gosint -v` prints the same line. A build from source without a version prints `devel`.

### gosint completion

```bash
gosint completion bash > /etc/bash_completion.d/gosint
gosint completion zsh > "${fpath[1]}/_gosint"
```

Prints the shell completion script for `bash`, `zsh`, `fish` or
`powershell`. `gosint completion <shell> --help` says how to load it.

### gosint help

`gosint help <command>` is the same as `gosint <command> --help`.

### Exit status

| Status | Meaning |
| --- | --- |
| `0` | Success |
| `1` | The command failed |
| `2` | A usage error: an unknown command or flag, a stray argument, an invalid configuration, an unseal key that is too short or does not open the server |

## gosint-desktop

```bash
gosint-desktop
```

The desktop app takes no command and no flag. It runs a node on the loopback,
on the port it used last time, and shows the console in a native window.
Closing the window stops the node. A second launch while one runs says gosint
is already running.

It reads `gosint.yaml` from its data directory and the `GOSINT_*` variables,
and never `./gosint.yaml` from the current directory:

| System | Data directory |
| --- | --- |
| macOS | `~/Library/Application Support/gosint` |
| Linux | `$XDG_DATA_HOME/gosint`, else `~/.local/share/gosint` |
| Windows | `%LOCALAPPDATA%\gosint` |

See [Desktop app](/getting-started/desktop-app/).
