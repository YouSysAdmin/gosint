---
title: Transforms
description: "Run a transform on a selection, give it parameters and secrets, follow the scan, and see how its findings land in the graph."
weight: 40
---

A transform takes entities of one type and finds new ones: a domain's DNS
records, an address's open ports, an email's breaches, a CVE's details.
What it finds is drawn into the sketch, linked to the entity it came from.

Every transform has an **input type** and an **output type**. Some write
several types (a WHOIS lookup gives an organization, emails and phones).
Some enrich in place: they take an entity and lay what they find over its
own fields, such as the reputation of an IP address.

The full list is in [Transforms](/reference/transforms/). The **Transforms**
page in the top bar lists the ones this server offers, including your
templates and the transform files an operator added, with the secrets each
needs.

## Running a transform

Select one entity, or several of the **same type**, then do one of these:

- right click and pick a transform from the **Transforms** tab of the menu
- press **Transform** in the selection bar, the entity panel or the left
  panel
- ask the [assistant](/guide/assistant/) to run one

The menu lists only the transforms that take that type. A gear marks a
transform with parameters, **key** one that needs a secret, **template**
a template, **file** a transform file. Picking one without parameters
starts it at once. Otherwise the run dialog opens.

The run dialog (**Run a transform on N domain**) has a filter, the
transforms, and under them the **Flows** that start from that type. Pick
one to see its description and its parameters, then press **Run**.

## Parameters

A parameter is one of these:

- a checkbox
- a choice from a list, such as `source` on `ip_to_ports`
- a number, a text or a URL
- a **secret**: the name of a key stored in your
  [Secrets](/guide/secrets/)

A secret parameter is filled with the name the transform expects, such as
`SHODAN_API_KEY`. If no secret by that name is stored, the dialog says so
and links to Secrets. You can also type the name of another secret you
stored. The value itself never leaves the server and never appears in a
scan or a log.

Parameters are checked before anything is queued: an unknown parameter or
a missing required secret is refused right away, naming the field.

## Scans and the live log

A run is a **scan**. It is queued, picked up by a worker on the server, and
logs as it goes. Open the console at the bottom of the sketch (**Cmd/Ctrl+D**)
to follow it:

- **Log** shows every line live. Each run starts by saying what it runs on
  and with which parameters (a secret by its name only). Narrow the log by
  level, or click a line to jump to its scan.
- **Scans** lists every run with its status: pending, running, completed,
  failed or cancelled. The badge on the tab counts what is running.

On a scan row:

- **log** shows only that scan's lines
- **found** (once completed) selects everything the scan found, zooms to it
  and marks it with a dashed ring. **unmark**, Escape or a click on the
  empty canvas drops the ring.
- **steps** (for a flow) shows what each step did
- **Cancel** stops a pending or running scan

When a scan settles, the graph reloads with what it found. The new
entities start beside the entity they came from.

A launch does not open the console, so you can keep working. The scans run
on the server: you can close the page and come back later.

## How results merge

A transform writes through the entity types, exactly as a hand edit does:

- An entity it finds again is **updated**, not duplicated. Its fields are
  merged key by key, so what an earlier transform found is kept.
- The first find is recorded as the entity's origin ("found by X from Y")
  and never changes. Each later find updates **Last seen**.
- A value its type refuses (an invalid address, a malformed CVE id) is
  logged and skipped. The rest of the run goes on.
- A relationship with the same label between the same two entities exists
  once.

To take back a run, press **found** on its scan and delete the selection.
Transform runs are not part of undo.

## Flags set by transforms

Some transforms colour an entity's flag to say something at a glance:
red for an address with a bad reputation or a domain that does not answer,
yellow for a site that resolves but does not answer 200, or for an address
behind a CDN. A later run that finds the entity clean clears the flag it
set.

## Filters

A **filter** is a transform that narrows a set instead of finding new
entities. Its description starts with "Narrows:". It takes and gives the
same type, and passes on only the entities that pass. They matter in a
[flow](/guide/flows/), where the next step runs only on what a filter kept:

- `ip_drop_cdn` drops addresses in CDN ranges (Cloudflare, Fastly, AWS,
  GCP)
- `ip_public_only` drops private addresses
- `domain_root_only` keeps root domains
- `domain_alive` keeps the domains that resolve, or that answer over HTTP

A dropped entity stays in the sketch, marked with a property and a flag.

## Passive and active sources

Most transforms ask a third party (crt.sh, RIPEstat, Shodan, VirusTotal)
and never touch the target. A few can contact the target directly:

- `ip_to_ports` reads Shodan InternetDB by default. With `source` set to
  **Active** it runs nmap on the server against the address, trying the
  most common ports (`top_ports`, at most 1000). nmap must be installed on
  the server.
- `domain_alive`, the crawlers and the site probes send requests to the
  sites the entities name.

Requests to sites an entity names never reach private, loopback or
link-local addresses, unless an administrator allows it for a lab (see
[Tools](/administration/tools/)).

## Your own transforms

Beyond the built-in ones you can write transforms as YAML templates in the
console, or an operator can drop template and script files into a
directory on the server. See
[Transform templates](/extending/transform-templates/) and
[Transform files](/extending/transform-files/).
