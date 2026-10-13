---
title: Installation
description: "Install the gosint server with the script, from a release archive, as a container image, or from source."
weight: 10
---

gosint is one binary. The API, the web console and the workers that run
transforms are all inside it. Pick the way that fits your machine, then go on
to [First run](/getting-started/first-run/).

If you want gosint on your own workstation in a window of its own, without a
server to run, see the [desktop app](/getting-started/desktop-app/) instead.

## Install script

On Linux (amd64 or arm64) and macOS (Apple Silicon) the install script
downloads the latest release for your machine, checks it against the
published checksums and installs the binary:

```bash
curl -sSfL https://raw.githubusercontent.com/yousysadmin/gosint/master/scripts/install.sh | sh
```

By default the binary goes to `~/.local/bin`. The script takes a few options:

| Option | What it does |
| --- | --- |
| `-b <dir>` | Installs into another directory. The `BINDIR` variable does the same. |
| `-d` | Prints debug output. |
| `<tag>` | Installs that release instead of the latest, for example `v1.2.0`. |

Options go after `sh -s --`:

```bash
curl -sSfL https://raw.githubusercontent.com/yousysadmin/gosint/master/scripts/install.sh | sh -s -- -b /usr/local/bin v1.2.0
```

Make sure the directory is on your `PATH`, then check the install:

```bash
gosint version
```

## Release archives

Every [release](https://github.com/YouSysAdmin/gosint/releases) carries an
archive per platform:

| Platform | Archive |
| --- | --- |
| Linux amd64 | `gosint_v<version>_linux_amd64.tar.gz` |
| Linux arm64 | `gosint_v<version>_linux_arm64.tar.gz` |
| macOS Apple Silicon | `gosint_v<version>_darwin_arm64.tar.gz` |
| Windows amd64 | `gosint_v<version>_windows_amd64.zip` |

Besides the binary, an archive holds `gosint.example.yaml` (the configuration
with every key explained), `examples/transforms` (two transforms of your own
to start from) and `examples/sketches` (a sample sketch to import). The
checksums of all archives are in `checksums.sha256`.

```bash
tar -xzf gosint_v1.2.0_linux_amd64.tar.gz
sha256sum --ignore-missing -c checksums.sha256
```

## Container image

The image is published on GitHub's registry for linux amd64 and arm64:

```bash
docker run -d --name gosint -p 4000:4000 -v gosint-data:/data ghcr.io/yousysadmin/gosint:latest
```

Then open <http://localhost:4000/>.

| Tag | Points at |
| --- | --- |
| `latest` | The newest stable release |
| `v<major>.<minor>` | The newest release of that series, for example `v1.2` |
| `<version>` | One release exactly, for example `1.2.0` |

A pre-release is only published under its own version, it never moves
`latest` or a series tag.

What the image sets up for you:

- The server listens on `0.0.0.0:4000` inside the container
  (`GOSINT_SERVER_ADDR`).
- The SQLite database is `/data/gosint.db`, and the files, the cache and the
  transforms directory sit beside it in `/data`. Keep `/data` on a volume.
- It runs as the unprivileged user `gosint` (uid 1000).
- `nmap` is installed, so the active port scan of `ip_to_ports` works.
- Python 3 with common libraries, `bash`, `curl`, `jq`, `whois`, `dig` and
  `exiftool` are installed for [transform files](/extending/transform-files/)
  that run a script.

Any configuration key can be passed as an environment variable, for example
`-e GOSINT_SERVER_PUBLIC_URL=https://osint.example.com`. See
[Configuration](/administration/configuration/).

{{< callout type="note" title="The server starts sealed" >}}
After every start, and in a container after every restart, the server waits
for its unseal key before it opens anything. See
[First run](/getting-started/first-run/) and [The vault](/administration/vault/).
{{< /callout >}}

## From source

You need Go (the version in `.go-version`), Node.js 22 with npm, and
[go-task](https://taskfile.dev).

```bash
git clone https://github.com/YouSysAdmin/gosint.git
cd gosint
task build
```

`task build` builds the web console and then the binary into `bin/gosint`,
with the console embedded. A plain `go build ./cmd/gosint` also compiles, but
without `task web` first the binary carries no console and `/app` serves
nothing.

## Next

[First run](/getting-started/first-run/): start the server, choose the unseal
key and create the administrator.
