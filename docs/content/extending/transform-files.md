---
title: Transform files
description: "Global transforms from a directory: a template file, or a script in any language."
weight: 20
---

The server reads transforms from a directory as well as from its own
code. A YAML file dropped there becomes a transform for every account,
launched by name like a built-in one, and it shows up within seconds,
without a restart. Two kinds of file are read:

- a **template**, one HTTP request per input entity, in the
  [template format](/extending/transform-templates/)
- a **manifest** with a `run` line, which runs a script of yours once
  per input entity, in any language

## The directory

`transforms.dir` in the configuration names it. Left empty:

- with SQLite, it is `transforms` beside the database file, so
  `data/transforms` for the default `data/gosint.db`
- with PostgreSQL, there is no directory unless you set one
- in the desktop app, it is `transforms` in the app's data directory

The server creates the directory at start.

```yaml
transforms:
  dir: /srv/gosint/transforms
```

The directory is read again when the transform list is asked for and
the last read is over two seconds old. Only files whose modification
time or size changed are parsed again, and a removed file goes away.
What is read:

- files ending in `.yaml` or `.yml`, at the top of the directory
- not hidden files (starting with `.`), not subdirectories

A file that does not parse is left out, and the reason goes to the
server log once per change of the file, as `transform file left out`
with the file name and the error. Fix the file and save it, the next
read picks it up.

{{< callout type="note" title="Every node needs the same files" >}}
A team install with several nodes reads the directory on each node,
and a scan runs on whichever node claims it. Copy the files to every
node, or point `transforms.dir` at a directory they share.
{{< /callout >}}

### Names

A file's `name` is the transform's name in the catalog, in flows and
in the API. The name of a built-in transform cannot be taken: a file
declaring one is left out and logged, and the built-in one stays. When
two files declare the same name, the one whose file name sorts first
is kept and the other is logged.

In the catalog a transform from a file carries a **file** badge.

## A template file

A template file is exactly what the console's template editor saves,
and what **Export** on a template downloads. Drop the YAML into the
directory as it is. As a file it is global: every account sees it and
runs it, each with their own secrets for the `secrets` it lists. See
[Transform templates](/extending/transform-templates/) for the format.

## A script

A file with a `run` line is a manifest. It describes a script as a
transform:

```yaml
name: domain_to_wayback
description: Snapshots of the domain in the Wayback Machine, as websites
category: Domain
input: domain
output: website
run: python3 domain_to_wayback.py
timeout: 90
params:
  - name: limit
    type: number
    description: How many snapshots to list, newest first
    default: "20"
```

### Manifest keys

| Key | Required | What it is |
| --- | --- | --- |
| `name` | yes | The transform's name: lowercase letters, digits, `-` and `_`, starting with a letter or a digit. |
| `description` | no | One line, shown in the run dialog and the catalog. |
| `category` | yes | The group it is listed under. |
| `icon` | no | An icon name for the catalog. |
| `input` | no | The entity type taken. Empty takes every type. |
| `output` | no | The type it mostly writes, used by the flow editor to fit steps together. Empty when it writes several. |
| `run` | yes | The command line. |
| `timeout` | no | Seconds one run may take, per input entity, 1 to 3600. 60 by default. |
| `params` | no | The parameters the run dialog asks for. |

Unknown keys are refused, so a misspelt one shows up in the log.

`run` is split on spaces, with no shell and no quoting. The first word
is the program, looked up on the `PATH`, or a path relative to the
transforms directory such as `./find.sh` (which must then be
executable). Keep spaces out of script names.

### Parameters

Each entry of `params`:

| Key | What it is |
| --- | --- |
| `name` | Letters, digits, `_`, `.` and `-`, each name once. |
| `type` | `string` (the default), `number`, `url`, `bool` (a checkbox, `"true"` or `"false"`), `select`, or `vaultSecret`. |
| `description` | Shown beside the input. |
| `required` | A required parameter left empty stops the launch. |
| `default` | The value the run dialog starts with, as a string. |
| `options` | For a `select`: a list of `label` and `value` pairs. A select needs at least one. |

A `vaultSecret` parameter names a secret of the account that runs the
transform. The script receives the secret's **value**, the person only
ever picks the name.

```yaml
params:
  - name: depth
    type: select
    options:
      - label: Shallow
        value: "1"
      - label: Deep
        value: "3"
  - name: EXAMPLE_API_KEY
    type: vaultSecret
    required: true
    description: Your key for api.example.com
```

## The script contract

The script runs **once per input entity**, in the transforms directory.

### Input on stdin

One JSON object:

```json
{
  "input": {
    "id": "0199e4b2-7c1e-7a51-9d1a-2f3c4b5a6d7e",
    "type": "domain",
    "label": "example.com",
    "properties": {"domain": "example.com"}
  },
  "params": {"limit": "20"}
}
```

`params` holds every parameter the manifest declares, each as a string,
empty when none was given and there is no default.

### Output on stdout

One JSON object per line, as soon as you have it. Each line is one of
three things.

**An entity**, linked from the input:

```json
{"type": "ip", "label": "203.0.113.9", "relation": "RESOLVES_TO"}
{"type": "website", "properties": {"url": "https://example.com/", "title": "Home"}, "relation": "HAS_WEBSITE"}
{"type": "individual", "label": "Jane Doe", "relation": "OWNS", "reverse": true}
```

| Key | What it is |
| --- | --- |
| `type` | The entity type. |
| `label` | The entity's value as one would type it. It fills the type's primary field when `properties` leaves that empty. |
| `properties` | Fields of the type, by name. |
| `relation` | The relationship label, `UPPER_SNAKE_CASE`. `RELATED_TO` when left out. |
| `reverse` | `true` draws the relationship from the new entity to the input instead. |

**An update** of the input itself, its fields laid over what it has:

```json
{"update": {"alive": true, "http_status": 200}}
```

**A log line** for the scan's log, `info` unless the level is
`warning`, `warn` or `error`:

```json
{"log": "checked 3 pages"}
{"log": "rate limited, try later", "level": "warning"}
```

Every entity and update goes through the entity types like a hand
edit. A value the type refuses is skipped with a warning in the scan's
log, and the next line is read. An entity found again is updated, not
duplicated, so a script may print the same thing twice. A line that is
not JSON, or that has no `type`, `update` or `log`, is a warning too.

One line may be up to 1 MB, and one run may print up to 16 MB. Past
that the script is stopped and the input fails.

### Exit, errors and time

- **Exit 0** is success.
- **Any other exit** fails this input. The last line the script wrote
  to stderr is in the scan's log with the exit status. Lines printed to
  stdout before the failure are kept.
- **The timeout** stops the script and fails this input. On Linux and
  macOS the script runs in its own process group and the whole group is
  killed, so a child it started does not outlive it. On Windows only
  the script itself is stopped.
- **Cancelling the scan** stops the running script and skips the inputs
  left.

A failed input never stops the others: the next one runs.

### Environment

The script does **not** inherit the server's environment. Only what a
process needs to start passes through: `PATH`, `HOME`, `USER`, `SHELL`,
`LANG`, `LANGUAGE`, every `LC_*`, `TMPDIR`, `TMP`, `TEMP` and `TZ`
(and on Windows `SYSTEMROOT`, `WINDIR`, `COMSPEC`, `PATHEXT`,
`USERPROFILE`, `APPDATA`, `LOCALAPPDATA`). The database address, the
vault settings, every `GOSINT_*` variable and any API key the server
was started with stay out. An API key a script needs is a
`vaultSecret` parameter.

### The sandbox

On Linux with Landlock (kernel 5.13 or newer) a script runs **confined**,
which is the default wherever the node supports it. It may:

- read and run the system: `/usr`, `/bin`, `/sbin`, `/lib*` and the
  entries of `/etc` a runtime needs (certificates, name resolution, the time
  zone, the dynamic linker, `passwd` and `group`), not `/etc` as a whole and
  not `/opt`, where an install of the server itself often sits,
- read its own directory, but not write to it,
- read and write a scratch directory of its own, made for the run and
  removed after it, which is its `HOME`, `TMPDIR`, `TMP` and `TEMP`, but
  not run a program it put there: what a script writes is data,
- reach the paths an administrator adds on the [Tools](/administration/tools/#transform-file-scripts)
  page, a wordlist or a database to read, a directory to write. A tool
  under `/opt` is such a path. A path reaching the server's own files (its
  config, the sqlite database, the unseal key file, the TLS pair) is refused.

Everything else is refused, the server's data directory, its config, the
unseal key file, the home of the server's user and `/proc` among them, so
a script cannot read the server's environment either. An interpreter or a
virtualenv outside the system trees, under a home directory for example,
has to be added as a path the script may read. The network is not limited.

A confined script that touches what it may not gets a permission error
from the system, which shows up in the stderr tail of the scan log. A
path added on the Tools page that this node does not have is reported
in the scan log before the script runs, and the server's log says at
start when the node cannot confine scripts at all, or when one of the
server's own files lies where every script may read (under `/usr`, say).

{{< callout type="warning" title="Without the sandbox, a script is code the server runs" >}}
On macOS, Windows and a Linux kernel without Landlock, a script runs as the
server's own user with its file system and its network, and can read
whatever that user can. That is a decision, not an oversight: serve gosint
on Linux, or accept that risk. If the sandbox is switched on where it cannot
work, scripts refuse to run instead of running unconfined. Either way, keep
the transforms directory writable by the people who run the server and
nobody else, and nothing guards where a script connects the way the
template client does.
{{< /callout >}}

## A complete example

The manifest above, `domain_to_wayback.yaml`, beside this script,
`domain_to_wayback.py`, which lists the snapshots the Wayback Machine
holds of a domain, newest first, using only the Python standard
library:

```python
#!/usr/bin/env python3
# Lists the Wayback Machine's snapshots of a domain, newest first, as
# website entities under ARCHIVED_AT.
import json
import sys
import urllib.parse
import urllib.request

CDX = "https://web.archive.org/cdx/search/cdx"


def main() -> None:
    doc = json.load(sys.stdin)
    domain = doc["input"]["properties"].get("domain") or doc["input"]["label"]
    limit = int(doc["params"].get("limit") or 20)

    query = urllib.parse.urlencode({
        "url": domain,
        "output": "json",
        "fl": "timestamp,original,statuscode",
        "filter": "statuscode:200",
        "collapse": "digest",
        "limit": -limit,
    })
    req = urllib.request.Request(f"{CDX}?{query}", headers={"User-Agent": "gosint-wayback/1.0"})
    with urllib.request.urlopen(req, timeout=60) as resp:
        rows = json.load(resp)

    seen = 0
    for ts, original, _ in rows[1:]:
        print(json.dumps({
            "type": "website",
            "properties": {
                "url": f"https://web.archive.org/web/{ts}/{original}",
                "title": f"{original} at {ts}",
            },
            "relation": "ARCHIVED_AT",
        }))
        seen += 1

    print(json.dumps({"log": f"{domain}: {seen} snapshots"}))


if __name__ == "__main__":
    main()
```

Copy both files into the transforms directory. Within a couple of
seconds `domain_to_wayback` is in the catalog of every account, under
Domain, with a `limit` field in its run dialog. Both files ship in the
release archives under `examples/transforms`, with a template file,
`ip_geolocation.yaml`, beside them.

A script in any language works the same way. A shell script can let
`jq` read the input and write the line, which also escapes the values:

```sh
#!/bin/sh
# Prints the input's label back as a phrase, to try the plumbing.
exec jq -c '{type: "phrase", label: ("seen " + .input.label), relation: "NOTED_AS"}'
```

## Debugging

- **The transform does not show up.** Look in the server log for
  `transform file left out` with the file's name. A YAML error, an
  unknown key, an unknown entity type, a missing `category` or `run`,
  and a built-in name are all reported there.
- **Run the script by hand**, from the transforms directory, with the
  input it would get:

  ```sh
  echo '{"input": {"id": "x", "type": "domain", "label": "example.com",
    "properties": {"domain": "example.com"}}, "params": {"limit": "5"}}' \
    | python3 domain_to_wayback.py
  ```

  Every line it prints should be one JSON object.
- **Read the scan's log.** Launch it from a sketch and open the Scans
  drawer. The run logs what it ran on and with which parameters, every
  entity it found, every refused value, every non-JSON line, and the
  last stderr line of a failed run.
- **Remember the environment is clean.** A script that works in your
  shell and fails under the server often reads a variable the server
  does not pass. Use a parameter instead.
