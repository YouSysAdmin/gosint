---
title: Transform templates
description: "A transform declared as YAML: one HTTP request per entity, the answer mapped onto entities."
weight: 10
---

A template is the quickest way to add a transform. It needs no code: it
describes one HTTP request, sent once for every input entity, and says
which parts of the answer become which fields of an entity. Most JSON
APIs fit in twenty lines.

Templates live in two places:

- **In the console**, under Transforms, "New transform". A template made
  there belongs to your account. You can share it with every account.
- **As files** in the server's transforms directory, global for every
  account. See [Transform files](/extending/transform-files/).

Both read the same format, described here.

## A first template

This one fills an `ip` entity with its country, city and ISP from
ip-api.com, which needs no key:

```yaml
# Same input and output type with no array: the mapped fields are
# laid over the input.
name: ip_geolocation
description: Country, city and ISP of an address from ip-api.com
category: IP
version: 1.0
input:
  type: ip
  key: address
request:
  method: GET
  url: "http://ip-api.com/json/{{address}}"
  params:
    fields: "status,country,city,lat,lon,isp"
response:
  expect: json
  map:
    country: country
    city: city
    latitude: lat
    longitude: lon
    isp: isp
output:
  type: ip
```

Run on `8.8.8.8`, it sends
`GET http://ip-api.com/json/8.8.8.8?fields=status%2Ccountry%2Ccity%2Clat%2Clon%2Cisp`,
reads the JSON answer, and writes `country`, `city`, `latitude`,
`longitude` and `isp` onto the address.

## The format

Every key is listed below. An unknown key is refused when the template
is saved, so a misspelt one is an error and never a setting silently
ignored.

### Top level

| Key | Required | What it is |
| --- | --- | --- |
| `name` | yes | The transform's name: lowercase letters, digits, `-` and `_`, up to 128 characters, starting with a letter or a digit. It may not be the name of a built-in transform. |
| `description` | no | One line, shown in the run dialog and the catalog. |
| `category` | yes | The group the transform is listed under, such as `IP` or `Domain`. Any text. |
| `version` | no | A number for your own bookkeeping. Nothing reads it. |
| `input` | yes | The entity type taken, and which of its fields fills the placeholder. |
| `request` | yes | The HTTP request. |
| `response` | yes | How to read the answer and which answer path fills which field. |
| `output` | yes | The entity type produced, and how it is linked. |
| `secrets` | no | The secrets the request reads. |
| `retry` | no | How a failed request is tried again. |

### input

| Key | Default | What it is |
| --- | --- | --- |
| `type` | | An entity type name, such as `domain`, `ip` or `email`. See [Entity types](/reference/entity-types/). A custom type works too. |
| `key` | `nodeLabel` | The field of the input entity whose value fills the `{{key}}` placeholder. `nodeLabel` (or `label`) is the entity's label. |

Use the type's primary field as the key, `domain` for a domain,
`address` for an ip, `email` for an email. An input entity that has no
value for the key is skipped with a warning in the log.

### request

| Key | Default | What it is |
| --- | --- | --- |
| `method` | `GET` | `GET` or `POST`. |
| `url` | | An `http` or `https` URL, with placeholders. |
| `params` | | Query parameters added to the URL, a map of name to value, with placeholders. |
| `headers` | | Request headers, a map of name to value, with placeholders. A name is letters, digits and `-`. |
| `body` | | The request body, with placeholders. |
| `timeout` | `30` | Seconds one request may take, from 1 to 300. |

### response

| Key | Default | What it is |
| --- | --- | --- |
| `expect` | `json` | `json`, `xml` or `text`. |
| `map` | | Output field to answer path. At least one entry. |

A **JSON path** is dot separated keys, and a number indexes an array:
`data.attributes.country`, `results.0.name`. A path that leads nowhere
leaves the field out, it is not an error.

An **XML path** is dot separated element names. Each name is looked up
among the direct children first, then anywhere below, so `country`
finds the first `<country>` in the document. The value is the element's
text, trimmed.

With `text`, the whole answer is one string. Map a field to the empty
path to take it: `text: ""`.

An answer is read up to 16 MB.

### output

| Key | Default | What it is |
| --- | --- | --- |
| `type` | | The entity type written. |
| `is_array` | `false` | Map each item of a JSON array to its own entity. |
| `array_path` | | Where the array is in the answer, a JSON path. Empty means the answer itself is the array. |
| `relation` | `RELATED_TO` | The label of the relationship from the input to each output, in `UPPER_SNAKE_CASE`. |

With `is_array`, the paths in `response.map` are read from each item,
not from the whole answer. An answer whose `array_path` is not an array
fails that input. Arrays are read from JSON only.

### secrets

A list of the secrets the template reads, each written as
`{{secrets.NAME}}` in the request:

| Key | Default | What it is |
| --- | --- | --- |
| `name` | | The secret's name, as the person stores it under Secrets. Letters, digits, `_`, `.` and `-`. |
| `required` | `true` | A required secret missing from the account's Secrets stops the launch before anything is queued. |
| `description` | | Shown beside the secret in the run dialog. |

The values come from the Secrets of the account that runs the
template, never from the template itself. A secret that is optional
and missing has no value, and a placeholder naming it fails the input,
so use an optional secret only where the request still makes sense
without it.

### retry

| Key | Default | What it is |
| --- | --- | --- |
| `max_retries` | `3` | How many times a failed request is sent again, 0 to 10. |
| `backoff_factor` | `0.5` | Seconds before the first retry, doubled each time, 0.1 to 10. |
| `retry_on_status` | `[429, 500, 502, 503, 504]` | The statuses that are retried. A connection error is retried too. |

One wait is at most a minute and all the waits of one request together
at most five minutes. Any other status of 400 or more fails the input
at once.

## Placeholders

A placeholder is a name between double braces, spaces allowed inside:
`{{domain}}`, `{{ secrets.API_KEY }}`. Two kinds of name exist:

- the input key, as `input.key` names it (`{{address}}`, `{{nodeLabel}}`)
- a secret, as `{{secrets.NAME}}`

A placeholder with no value is an error for that input, never an empty
string.

Each value is escaped for the place it lands in, so a value read off
the network cannot change the shape of the request:

| Where | How the value is written |
| --- | --- |
| `url` | Percent-encoded except letters, digits and `-_.~`. A `/` in the value becomes `%2F`, so a placeholder fills one path segment or one query value. |
| `params` | Written as is, then the whole query is URL-encoded. |
| `headers` | Written as is. |
| `body` | By the body's kind: as the inside of a JSON string, a form value, or XML text. |

The body's kind comes from its `Content-Type` header when there is
one: `application/json` or any `+json` type, `application/x-www-form-urlencoded`,
or a type ending in `xml` and `text/html`. Without the header it is
told from the body itself: one starting with `{` or `[` is JSON, one
starting with `<` is XML, one shaped like `name=value&name=value` is a
form. Anything else is written as is.

In a JSON body, put the quotes around the placeholder yourself. The
escaping writes what goes between them:

```yaml
body: '{"query": "{{email}}", "type": "email"}'
```

## In place, or new entities

When the input and output types are the same and `is_array` is off, the
template **enriches its input**: the mapped fields are laid over the
input's own properties and written back. The answer does not need to
repeat the input's primary field. The geolocation template above works
this way.

Otherwise every mapped answer is a **new entity**, linked from the
input by `output.relation`. Writing an entity that exists already
updates it, so running a template twice adds nothing twice.

Every value goes through the output type like a hand edit. A value the
type refuses (a malformed address, a field of the wrong kind) is
skipped with a warning in the scan's log, and the rest goes on.

## More examples

A list of subdomains behind an API key, each written as a domain under
`HAS_SUBDOMAIN`. The input and output are both `domain`, but with
`is_array` set the results are new entities:

```yaml
name: domain_to_subdomains_example
description: Subdomains of a domain from a lookup API
category: Domain
input:
  type: domain
  key: domain
request:
  method: GET
  url: "https://api.example.com/v1/domain/{{domain}}/subdomains"
  headers:
    X-API-Key: "{{secrets.EXAMPLE_API_KEY}}"
  params:
    limit: "100"
response:
  expect: json
  map:
    domain: hostname
output:
  type: domain
  is_array: true
  array_path: data.subdomains
  relation: HAS_SUBDOMAIN
secrets:
  - name: EXAMPLE_API_KEY
    description: Your key for api.example.com
```

A POST with a JSON body, the usernames an address is registered under:

```yaml
name: email_to_usernames_example
description: Usernames registered with an address
category: Email
input:
  type: email
  key: email
request:
  method: POST
  url: "https://api.example.com/v2/search"
  headers:
    Content-Type: application/json
    Authorization: "Bearer {{secrets.EXAMPLE_TOKEN}}"
  body: '{"query": "{{email}}", "type": "email"}'
  timeout: 60
response:
  expect: json
  map:
    value: username
    platform: site
output:
  type: username
  is_array: true
  array_path: results
  relation: HAS_USERNAME
secrets:
  - name: EXAMPLE_TOKEN
```

A plain text answer, an API that answers one line, kept as a phrase:

```yaml
name: ip_to_reverse_note_example
description: A one line note about an address
category: IP
input:
  type: ip
  key: address
request:
  url: "https://api.example.com/note/{{address}}"
response:
  expect: text
  map:
    text: ""
output:
  type: phrase
  relation: HAS_NOTE
```

## Where requests may go

Template requests go through the same guarded client as the transforms
that visit sites an entity names. It refuses loopback, private,
link-local and carrier-grade NAT addresses at connect time, which also
covers a redirect or a DNS answer pointing inward. A lab that needs
private targets has a switch for it, which an administrator sets on the
[Tools](/administration/tools/) page.

## Writing one in the console

Transforms, **New transform** opens the editor:

- The **Input** and **Output** selects and the YAML stay in step both
  ways. Picking a type rewrites the `type` of the block, sets `input.key`
  to the type's primary field (the `{{...}}` placeholders follow), and
  adds a `# fields:` comment listing the type's fields, replaced on the
  next pick. Typing a type in the YAML moves the select. An inline block
  such as `input: {type: ip}` is read but left as written.
- **Draft it with the model** asks the assistant for a template from a
  sentence ("Look an IP up on ip-api.com and keep its city, country and
  ISP"). The draft replaces the editor and follows its Input and Output.
  When the draft does not parse, the reason is shown above the editor so
  you can finish it by hand. Read it before saving, a model guesses
  field names. An administrator has to have added a model first, see
  [models](/administration/models/).
- **Save** checks the template. A refusal names every problem at once.
- **Try it** (after the first save) runs the template on one value you
  type, with your own secrets, sends the request once without retries,
  and shows the entities it would write and the raw answer. Nothing is
  written to any sketch.

A saved template is launched from a sketch like any transform, see
[Transforms](/guide/transforms/).

## Sharing

A template is private to its owner unless "Share with every account"
is ticked. A shared template shows up in every account's catalog, and
only its owner can change or delete it.

{{< callout type="warning" title="A shared template that reads secrets runs for its owner only" >}}
A template's request goes wherever its author wrote, and the secrets it
reads come from the account that runs it. If anyone could run a shared
template that reads secrets, its author could collect other people's
API keys at an address of their choosing. So such a template is refused
for everyone but its owner. A shared template with no `secrets` runs
for every account.
{{< /callout >}}

## Export and import

A template moves as its YAML:

- **Export** on a template's row downloads it as `<name>.yaml`.
- **Export my templates** downloads every template you own as one file,
  the documents separated by `---` lines.
- **Import templates** reads such a file, one or many documents. Each is
  checked like a save and becomes your own private template. A document
  that does not parse, one named like a built-in transform, or one
  named like a template you already have is skipped and listed.

A file exported here can also be dropped into the server's transforms
directory as it is, to make the template global. See
[Transform files](/extending/transform-files/).
