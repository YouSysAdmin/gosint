# Gosint

Self-hosted OSINT graph exploration.

![A sketch in the console: the graph of an investigation with a finding selected](docs/screenshot.png)


## IMPORTANT
If you forget your **unseal key**, your data cannot be recovered.

The `./skills` catalog includes the skill that will simplify a development of transformers for a services you need (though, most likely, the official models will refuse to do that for you :) ).

## Features

**Investigations and graphs**

- Investigations with members and roles (owner, admin, editor, viewer),
  invitations by link, an admin panel for accounts.
- Sketches as graph canvases of typed entities: 48 entity types with
  detection from text, custom types declared by an administrator,
  custom fields on any entity.
- Provenance on every entity: who or which scan found it, and when it
  was last seen.
- Undo and redo of hand edits, with deleted files kept for a while so
  an undo brings them back.
- Live edits: what a teammate changes shows up in the open sketch.
- Search across sketches, findings and vulnerabilities, a map view,
  table and relationship views with CSV export.
- Written analyses with entity mentions, autosaved.

**Transforms and flows**

- 56 built-in transforms: DNS, WHOIS, certificates, subdomains, ASN
  and CIDR, ports (Shodan InternetDB, or nmap on the server),
  reputation (AbuseIPDB, GreyNoise, VirusTotal, Censys, SecurityTrails,
  Hunter, urlscan), breaches (DeHashed), infostealers (HudsonRock),
  usernames across sites, crypto wallets, CVEs from the NVD with EPSS
  and KEV, geocoding, and the entities an entity's own fields name.
- Filters that narrow a set between flow steps: drop CDN ranges,
  private addresses, dead hosts, non-root domains.
- YAML template transforms, drafted by the assistant from a
  description.
- Your own transforms as files the server picks up while running: a
  template, or a script in any language (see `examples/transforms`).
- Flows chaining transforms in a tree, edited on a canvas, run by hand
  or on a schedule with a diff of what appeared and what went missing.
- A job queue on the database with live logs over SSE, no Redis.

**Import and export**

- nmap XML and nuclei reports as entities and findings, with a hole
  since the last scan marked.
- CSV and XLSX by a column mapping, plain text with type detection.
- A sketch or a whole investigation as one file, optionally encrypted
  under a passphrase, files included.
- Flows and templates as files.
- A sample sketch to start from in `examples/sketches`.

**Files and secrets**

- Files attached to entities, stored on disk, in S3 or in the
  database, with a viewer for images, PDF, audio, video, text and
  `.eml` messages. EXIF, PDF and Office metadata fill the entity's
  fields at upload.
- Secrets for API keys, kept per account, never shown again.
- Everything is encrypted at rest: the graph, analyses, chats, scans,
  logs, the files themselves, accounts, flows, templates and secret
  names. Without the unseal key the database is opaque bytes. The key
  is wrapped under it and the server starts sealed, opening nothing
  until it is entered. A label is still found by any piece of it,
  through a keyed index.

**Assistant**

- A chat about the selected entities on any OpenAI or Anthropic style
  model, LM Studio by default. It looks things up, runs transforms
  when the person could, and proposes graph changes to apply, never
  editing by itself.

**Accounts**

- Password sign-in with lockout, TOTP, recovery codes, passkeys, OIDC
  providers set up in the admin panel, password reset by mail.

## Storage

| Install | Backend | Notes |
|---|---|---|
| Local, single user | SQLite (default) | one file, `data/gosint.db`, nothing else to run |
| Team | PostgreSQL | several nodes may share one database |

Both run the same queries and the same schema, kept in step by
`migrations/postgres` and `migrations/sqlite`.

## Install

A release binary for linux (amd64, arm64), macOS (arm64) or Windows
(amd64) is on the [releases page](https://github.com/yousysadmin/gosint/releases).
On linux and macOS the script downloads the one for this machine,
verifies its checksum and puts it in `~/.local/bin` (or `-b DIR`):

```sh
curl -sSfL https://raw.githubusercontent.com/yousysadmin/gosint/master/scripts/install.sh | sh
```

As a container, with the database and the files under the volume:

```sh
docker run -d -p 4000:4000 -v gosint-data:/data ghcr.io/yousysadmin/gosint
```

Or with `compose.yaml`, one sqlite node on the loopback for a reverse
proxy on the same host:

```sh
GOSINT_PUBLIC_URL=https://osint.example.com docker compose up -d
```

### The seal

The server starts sealed. The first time, open the console and choose
an unseal key (or run `gosint vault init`), after every restart enter
it again on the same page. It cannot be recovered: without it nobody
can read the data, the operator included. `gosint vault rekey` changes
it. An unattended start may read it from `vault.unseal_key_file` or
`GOSINT_VAULT_UNSEAL_KEY`, which leaves the data only as safe as that
file. Every node of a team install is unsealed on its own.

### Recommended tools outside the container

The binary needs nothing else to run. Built-in transforms and template
transforms work as they are. Two things call programs on the server:

- `ip_to_ports` with `source: nmap` runs the `nmap` binary.
- Transform files with a `run` line (see `examples/transforms`) run
  whatever script the operator drops in.

The container ships the set below, so a script written against it runs
there as it is. On a host install, the same set:

| Kind | Tools |
|---|---|
| Scanner | `nmap` |
| Shell | `bash`, `curl`, `jq`, `yq` (mikefarah v4), `openssl`, `git` |
| Network | `dig` and `host` (bind tools), `whois` |
| Files | `exiftool` |
| Python 3 | `python3` with the packages below |

Python packages: `requests`, `httpx`, `aiohttp`, `PySocks`,
`beautifulsoup4`, `lxml`, `html5lib`, `feedparser`, `dnspython`,
`tldextract`, `netaddr`, `whois`, `phonenumbers`, `validators`,
`python-dateutil`, `pytz`, `PyYAML`, `PyJWT`, `cryptography`, `Pillow`,
`ExifRead`, `maxminddb`, `geoip2`, `shodan`, `ipwhois`.

Debian or Ubuntu:

```sh
sudo apt install nmap bash curl jq openssl git bind9-dnsutils whois \
  libimage-exiftool-perl python3 python3-venv
# yq: https://github.com/mikefarah/yq/releases
python3 -m venv ~/.local/share/gosint/python
~/.local/share/gosint/python/bin/pip install requests httpx aiohttp PySocks \
  beautifulsoup4 lxml html5lib feedparser dnspython tldextract netaddr whois \
  phonenumbers validators python-dateutil pytz PyYAML PyJWT cryptography \
  Pillow ExifRead maxminddb geoip2 shodan ipwhois
```

macOS:

```sh
brew install nmap bash jq yq bind whois exiftool python
```

then the same venv and `pip install` as above.

A script inherits only `PATH`, `HOME`, the locale, the temp dir and
the zone from the server, so put the venv's `bin` first on the
server's `PATH`, or name its interpreter in the manifest's `run` line.

## Run

```sh
task build          # console + bin/gosint
./bin/gosint serve  # http://127.0.0.1:4000
```

Against PostgreSQL:

```sh
GOSINT_DATABASE_DRIVER=postgres \
GOSINT_DATABASE_DSN='postgres://user:pass@host:5432/gosint?sslmode=require' \
./bin/gosint serve
```

Migrations are applied at boot. On PostgreSQL the run holds an advisory lock,
so nodes starting together apply them once. `serve --skip-migrate` refuses to
start on an outdated schema instead.

Configuration: `gosint.yaml` (see `gosint.example.yaml`) or `GOSINT_*`
environment variables.

## Develop

```sh
task dev-up     # postgres on :5434 for the team backend
task run        # server on sqlite
task web-dev    # vite on :5173, proxying /api
task check      # vet, lint, tests, gofmt, type-check, prettier
task test-db    # tests on both backends
lefthook install  # git hooks: the checks on what a commit touches
```

A release is a GitHub release on a `vX.Y.Z` tag: the workflow runs
goreleaser, which builds the console, the binaries, the archives and the
image on `ghcr.io/yousysadmin/gosint`. A tag with a pre-release part
(`v1.2.0-rc.1`) moves no `latest` tag.
