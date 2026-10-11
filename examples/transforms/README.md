# Transform files

Copy a file from here into the server's transforms directory (beside
the sqlite database by default, `transforms.dir` in the config) and it
shows up in the catalog within seconds, no restart, for every account.
A file that does not parse is left out and the reason is in the server
log. A file naming a built-in transform is left out too.

Two kinds of file, both YAML:

- A template: one HTTP request per input entity, the answer mapped
  onto an entity. The same format the console's "New transform" editor
  writes, see `ip_geolocation.yaml`.
- A manifest with a `run` line: a script run once per input entity, in
  this directory, see `domain_to_wayback.yaml` and `domain_to_wayback.py`.

## The script contract

The script reads one JSON object on stdin:

```json
{"input": {"id": "...", "type": "domain", "label": "example.com", "properties": {"domain": "example.com"}},
 "params": {"limit": "20"}}
```

`params` holds every parameter the manifest declares, a `vaultSecret`
one resolved to the runner's secret value. The server's own environment
variables (`GOSINT_*`) are not passed on.

It prints JSON lines on stdout, each one of:

- An entity linked from the input:
  `{"type": "ip", "label": "203.0.113.9", "relation": "RESOLVES_TO"}`.
  `properties` may replace or complete `label`, `relation` is
  `RELATED_TO` unless given, `"reverse": true` draws the edge from the
  entity to the input instead.
- Properties laid over the input: `{"update": {"alive": true}}`.
- A log line: `{"log": "checked 3 pages", "level": "info"}` (or
  `warning`).

Everything goes through the entity types like a hand edit: a value the
type refuses is logged and skipped. stderr goes to the log when the
script fails. A non-zero exit or the `timeout` (60 seconds by default)
fails that input only, the next one runs.

A team install reads the directory on every node, so copy the files to
each.
