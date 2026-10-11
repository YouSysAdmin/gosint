---
name: gosint-transform
description: Write gosint transform FILES - a YAML template (one HTTP request per entity, the answer mapped onto an entity) or a YAML manifest with a run line plus a script (python, shell, anything) speaking JSON lines. Use when asked to write, port, fix or review a gosint transform, transformer, template or script transform for a service, API, registry or open data source. Works without the gosint source code, against any gosint server. Not for built-in transforms written in Go inside gosint itself.
---

# Writing gosint transform files

gosint is a self-hosted OSINT graph tool: a sketch holds typed entities
(domain, ip, organization, individual...) and a TRANSFORM expands an
entity into new ones or fills it in. Besides the built-in transforms, a
server reads transform FILES from its transforms directory and offers
them to every account within 2 seconds, no restart. Two kinds:

- TEMPLATE: YAML with no `run` line. One HTTP request per input entity,
  the answer mapped onto an output type. Pick it when one request
  answers the question and the answer maps field by field.
- MANIFEST + SCRIPT: YAML with a `run` line, plus the program it runs
  once per input entity, printing JSON lines. Pick it for anything
  else: several requests, paging, polling, a captcha check, a choice
  between filling the input and listing matches, several entity types
  out, a proxy, a local binary.

Everything needed is in this skill folder. The gosint source is NOT
needed and is not assumed to be there.

## The skill's files

```
SKILL.md                     this file
references/entity-types.md   every built-in entity type, its primary field and fields
assets/skeleton.py           a stdlib-only script to start from
scripts/validate.py          checks files as the server does, runs a script as the server does
```

When the skill was given as a LINK, fetch the other three files next to
SKILL.md before starting, into a working folder (the scratchpad when
there is one). Turn a GitHub page link into raw file links:
`https://github.com/<owner>/<repo>/tree/<ref>/<path>` (or `/blob/.../SKILL.md`)
becomes `https://raw.githubusercontent.com/<owner>/<repo>/<ref>/<path>/<file>`.
The upstream copy is
`https://raw.githubusercontent.com/YouSysAdmin/gosint/master/skills/gosint-transform/`.
Read `references/entity-types.md` before choosing types. Never guess a
type's fields.

`validate.py` needs Python 3.10+ and PyYAML (`pip install pyyaml`).

## Workflow

1. Ask, unless already said: the source (URL, docs), what goes in and
   what should come out, where the files go (a folder of the user's or
   the server's transforms directory), and any custom entity types the
   server has (Admin, Types in the console) that the transform uses.
2. Find what the source answers: call it once (`curl`) and keep the
   real answer in front of you. Never map paths from memory.
3. Pick the input and output types from `references/entity-types.md`.
   Know each one's PRIMARY field, it is the identity (see Identity).
4. Write the file(s). Start a script from `assets/skeleton.py`.
5. Check: `python3 scripts/validate.py <dir or files>` prints `ok` or
   `FAIL` per file with the server's reason, and warnings for what
   would fail only at run time. Add `--type <name>` per custom type.
6. Run a script as the server would (its directory, the server's
   scrubbed environment, the stdin document, the timeout), seeing what
   the server would make of each line:
   ```sh
   python3 scripts/validate.py --run my_transform.yaml \
     --input '{"type": "domain", "label": "example.com", "properties": {"domain": "example.com"}}' \
     --param limit=5
   ```
   A `vaultSecret` param not given with `--param` is read from the
   environment variable of its name. A template is tried in the
   console instead (step 7).
7. Deploy (see Deploying) and run it on an entity in a sketch, then
   read the sketch's log.

## Deploying

The server reads its transforms directory FLAT: subdirectories are
ignored, so a collection is copied in with its helper modules side by
side. Where the directory is:

- binary install with sqlite: `transforms` beside the database file,
  or the `transforms.dir` setting of the config file.
- the Docker image: `/data/transforms` (the `/data` volume).
- postgres: only when `transforms.dir` is set.
- several nodes: every node needs the same files.

A file that does not parse is left out and the reason is in the server
log (`transform file left out`). A template can also be pasted into the
console (Transforms, New transform), where Test runs it on a typed
value. Saved there it belongs to the account instead of the server.

The Docker image's `python3` has: requests, httpx, aiohttp, PySocks,
beautifulsoup4, lxml, html5lib, feedparser, dnspython, tldextract,
netaddr, python-whois, phonenumbers, validators, dateutil, pytz, PyYAML,
PyJWT, cryptography, Pillow, exifread, maxminddb, geoip2, shodan,
ipwhois. Its tools: bash, curl, jq, yq, openssl, dig, whois, git,
exiftool, nmap. Anything else, or a binary install, means stdlib only
unless the user says the packages are there.

## Rules both kinds share

- `name`: `^[a-z0-9][a-z0-9_-]{0,127}$`, by convention
  `<input>_to_<what>` or `<prefix>_<source>`. Never a built-in
  transform's name (`validate.py` knows them, the file is refused) and
  unique across the directory (the second file is refused).
- `category` is required, the catalog groups by it (Domain, IP,
  Organization, or a collection's own).
- `description` is one line saying what comes out and from where,
  shown in the catalog and the run dialog.
- Unknown keys are REFUSED, so a typo is an error, not a silent
  default. A key given twice is refused. A number field refuses a
  quoted value (`timeout: "90"` fails), a text field takes any scalar.
- Relations are UPPER_SNAKE_CASE (`^[A-Z][A-Z0-9_]{0,63}$`),
  `RELATED_TO` when not given. Reuse gosint's vocabulary where it
  fits: `RESOLVES_TO`, `HAS_SUBDOMAIN`, `HAS_EMAIL`, `HAS_WEBSITE`,
  `HAS_PORT`, `REGISTERED_AT`, `DIRECTOR_OF`, `WORKS_FOR`,
  `HAS_SOCIAL_ACCOUNT`, `FOUND_IN_BREACH`, `LEAKED_WITH`,
  `ARCHIVED_AT`, `REFERENCES`.
- Everything written goes through the entity type like a hand edit: a
  value the type refuses is logged and that entity skipped, the run
  goes on. Unknown property keys are kept as custom fields (a
  `company_number` or `vat_id` on an organization), so a source's own identifiers need no
  new type.
- Properties MERGE key by key, never replace: writing an entity again
  adds and overwrites keys, it never erases one an earlier run found.
- Provenance (who found what, when) is written by the server, never
  put it in properties.
- Texts a person reads (description, param descriptions, log lines)
  never contain `;`. Code comments are ASCII and say what the code is
  for.
- Nothing a source returned is held back (passwords, hashes, personal
  data): privacy is the investigator's call, the server encrypts the
  graph at rest.

## Identity: the trap

An entity is `(type, primary field value)`, the label is display only.
Writing the input type with a DIFFERENT primary value makes or merges
ANOTHER entity, it does not rename this one. So an in-place update
keeps the primary field as it is: an organization's registry name goes
into `aliases` or `legal_name`, never into `name`. To offer several
candidates instead of guessing, emit them as new entities of the input
type under a relation such as `REGISTRY_MATCH`, and log that the person
should run it again on the right one. `validate.py --run` flags an
update that changes the primary field.

## Template format

```yaml
name: ip_geolocation
description: Country, city and ISP of an address from ip-api.com
category: IP
version: 1.0                  # optional, a number
input:                        # a MAPPING here, a plain name in a manifest
  type: ip
  key: address                # the property filling {{address}}, default nodeLabel (the label)
request:
  method: GET                 # GET or POST
  url: "http://ip-api.com/json/{{address}}"
  params:                     # the query string
    fields: "status,country,city,lat,lon,isp"
  headers:
    X-Key: "{{secrets.IPAPI_KEY}}"
  body: ""                    # see escaping
  timeout: 30                 # seconds, 1 to 300
response:
  expect: json                # json, xml or text
  map:                        # OUTPUT FIELD: ANSWER PATH
    country: country
    city: city
    latitude: lat
    longitude: lon
output:
  type: ip
  is_array: false
  array_path: ""              # with is_array, the dot path to the list
  relation: RELATED_TO
secrets:                      # each one is {{secrets.NAME}}
  - name: IPAPI_KEY
    required: false           # true unless set false
    description: ip-api pro key, the free endpoint without it
retry:                        # the defaults, a block given sets BOTH numbers
  max_retries: 3              # 0 to 10
  backoff_factor: 0.5         # seconds, doubled per attempt, 0.1 to 10
  retry_on_status: [429, 500, 502, 503, 504]
```

- Placeholders are `{{name}}`: the ONE input key (named as `input.key`,
  or `{{nodeLabel}}` / `{{label}}` for the label) and
  `{{secrets.NAME}}` for a declared secret. No other values exist: a
  placeholder with no value fails every input, so does an input
  lacking the key property.
- A secret is looked up among the RUNNING account's secrets (the
  console's Secrets page): the one the person picks at launch, else the
  one named like the secret. A public template that reads secrets runs
  for its owner only.
- Escaping is automatic: in the URL as a path component (everything
  but `A-Za-z0-9-_.~` percent-encoded), in `params` by the query
  encoder, in a body by its `Content-Type` header or its shape (JSON
  string escape, form encoding, XML escape). Write `"{{domain}}"` with
  the quotes inside a JSON body. Never pre-escape.
- `map` paths are dot paths, a number indexes an array
  (`data.items.0.name`). With `is_array: true` the paths are relative
  to each item of the list at `array_path` (`""` when the answer itself
  is the list), each item one entity. With `expect: text` the path
  `""` is the whole body. With `expect: xml` a path is element names, a
  direct child first, else the first descendant of that name, and the
  answer makes one entity.
- SAME input and output type and no array = IN PLACE: the mapped fields
  are laid over the input, which keeps its primary field. Otherwise
  each mapping is a new entity linked from the input by
  `output.relation`: map the output type's PRIMARY field, or the entity
  is keyed by whatever label its type computes, and refused when none.
- A non-2xx answer fails that input. A status in `retry_on_status` is
  retried with backoff, a minute per wait and 5 minutes in all at most.
- Requests go through the server's guarded client: no loopback,
  private, link-local or CGNAT targets unless the administrator allows
  private addresses (Admin, Tools). Answers are capped at 16 MB.
- One request per input only. Paging, two-step lookups or a decision
  on the answer means a script.

## Manifest format

```yaml
name: uk_companies_house
description: UK Companies House by company number or name - fills the organization, links its officers and address
category: Organization
icon: building-2              # optional, a Lucide icon name
input: organization           # a type name, "" takes any type
output: organization          # what it mostly writes, "" when mixed
run: python3 uk_companies_house.py   # split on spaces, run in the transforms directory
timeout: 90                   # seconds PER INPUT, 1 to 3600, default 60
params:
  - name: COMPANIES_HOUSE_API_KEY
    type: vaultSecret
    required: true
    description: Companies House API key, the secret named COMPANIES_HOUSE_API_KEY when none is picked
  - name: PROXY_URL
    type: vaultSecret
    description: Secret holding a proxy URL (socks5h://user:pass@host:1080), PROXY_URL when none is picked, direct without
  - name: limit
    type: number              # string (default), url, number, select, bool, vaultSecret
    description: How many matches to list on a search by name
    default: "20"
    required: false
  - name: mode
    type: select
    default: passive
    options:
      - {label: Passive, value: passive}
      - {label: Active, value: active}
```

- `run` is NOT a shell: no pipes, quotes, `~`, `$VAR` or `&&`. For
  `./find.sh` the file needs its exec bit and a shebang, otherwise
  name the interpreter (`python3 x.py`, `bash x.sh`). The working
  directory is the transforms directory, so `from my_common import ...`
  works for a module beside the script.
- The environment is SCRUBBED: only PATH, HOME, USER, SHELL, LANG,
  LANGUAGE, LC_*, TMPDIR, TMP, TEMP and TZ pass. No `HTTP_PROXY`, no API
  key from the environment. Anything the script needs comes in as a
  param.
- A key or a proxy is a `vaultSecret` param, never a `string` one and
  never in a file. The script gets the secret's VALUE. When the person
  picks no secret at launch, the secret NAMED LIKE THE PARAM is used,
  so name the param as a person would name the secret
  (`COMPANIES_HOUSE_API_KEY`, `PROXY_URL`), and say so in its description. A
  `default` on a vaultSecret is never read. With no secret at all the
  script gets `""`. Set `required: true` only when the run is useless
  without it: a missing required secret refuses the launch.
- `bool` params arrive as `"true"` / `"false"`, `number` as text.

### The script contract

stdin is one JSON object:

```json
{"input": {"id": "...", "type": "domain", "label": "example.com", "properties": {"domain": "example.com"}},
 "params": {"limit": "20", "PROXY_URL": "socks5h://..."}}
```

Every declared param is there as a STRING, its default applied, `""`
when it has neither a value nor a default. Read the input's primary
field first, the label as fallback, and other properties another
transform or a hand edit may have filled.

stdout is JSON lines, each one of:

| line | effect |
|---|---|
| `{"type": "ip", "properties": {"address": "203.0.113.9"}, "relation": "RESOLVES_TO"}` | an entity linked input -> entity |
| the same with `"reverse": true` | linked entity -> input (`individual -DIRECTOR_OF-> organization`) |
| `{"type": "ip", "label": "203.0.113.9"}` | `label` fills the primary field when the properties leave it empty |
| `{"update": {"alive": true}}` | laid over the input's properties (see Identity) |
| `{"log": "3 pages read", "level": "info"}` | a log line, `warning` / `warn` / `error` log as a warning |

- Lines are handled AS THEY COME: flush each one
  (`print(..., flush=True)`), and what was printed before a crash stays
  written.
- A line that is not JSON, or names no type, update or log, is a
  warning and the rest goes on. A line is at most 1 MB, a run 16 MB.
- `json.dumps(..., ensure_ascii=False)` keeps names in other scripts
  readable.
- Failure: the reason on stderr and a non-zero exit. The LAST stderr
  line lands in the sketch log, so make it one line saying what went
  wrong, prefixed with the source's name. Never print a secret, or a
  URL carrying one, there: every member reads the log. A failure or the
  timeout fails that input only, the next one runs.
- Log a one-line summary per input ("example.com: 12 snapshots"), a
  warning when the input lacks what the source needs ("no company
  number on the entity, run uk_companies_house first"), and an info line rather than silence
  when nothing was found.
- Be polite to the source: a User-Agent naming the tool, a timeout on
  every request, retries only on network errors, 429 and 5xx with a
  growing sleep, the source's rate limit honoured (a sleep between
  requests is fine, the timeout is per input).
- Nested values: a list field is a JSON array, a `ref:` field an
  object (`{"address": "..."}`) or, for `location`, a plain address
  line. Prefer emitting the referenced thing as its own entity with a
  relation, the graph shows it then.

## A collection

Several scripts against one family of sources share a module beside
them (a `<prefix>_common.py` with `read_input`, `emit`, `log`, a session
through the optional proxy, `request` with retries, a function pulling
the source's identifiers out of an entity). One manifest per transform,
the module has none. Prefix every name (`uk_`) and use one category.
Write a short README for the collection: what each transform does, the
secrets to add, the packages the scripts need.

Worked examples, a script and a template:
`https://github.com/YouSysAdmin/gosint/tree/master/examples/transforms`.

## Maintaining this skill (gosint repository only)

The formats follow `internal/core/template` and
`internal/core/transformdir` in the gosint repository. When they
change, update this file and `validate.py`'s checks. When entity types
or built-in transforms change, run `go run ./skills/gosint-transform/scripts/gen`
from the repository root: it rewrites the catalog inside `validate.py`
and the table in `references/entity-types.md`.
