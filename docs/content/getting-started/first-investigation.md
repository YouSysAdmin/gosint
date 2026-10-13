---
title: Your first investigation
description: "A short tour: an investigation, a sketch, an entity, a transform run on it, and an export."
weight: 30
---

This tour takes five minutes. It assumes the server is unsealed and you are
signed in. See [First run](/getting-started/first-run/) if not.

## Create an investigation

An **investigation** is one case: it holds the sketches, the written analyses
and the people working on it.

On the **Investigations** page press **New investigation**, give it a name and
an optional description, and press **Create**. You are its owner. Members and
their roles are covered in [Investigations](/guide/investigations/).

## Create a sketch

A **sketch** is one graph of entities inside an investigation. Open the
investigation, press **New sketch**, give it a title and press **Create**. The
sketch opens on an empty canvas.

## Add an entity

There are a few ways to put something on the canvas:

- **Double-click the empty canvas** and type a value, for example
  `example.com`. Its type is recognized as you type, and Enter adds it where
  you clicked.
- Press **Add** in the header to pick the type and fill its fields in a form.
- Press **Import** and paste values, one per line. Press
  **Recognize N values**, check the types gosint found, and press
  **Import N**.

Every value gets a type: a domain, an IP address, an email address, a
person's name, a phone number, a crypto wallet and many more. Text that
matches no type becomes a `phrase`. The full list is in
[Entity types](/reference/entity-types/).

Click an entity to open its panel on the right, with its properties,
relationships, notes, files and style. See [Entities](/guide/entities/).

## Run a transform

A **transform** takes entities of one type and finds new ones. Right-click the
`example.com` entity. The menu lists the transforms that take a domain, with a
search box on top. Pick `domain_to_ip`.

A transform with nothing to ask starts at once and a notice says so. One with
parameters opens a dialog first, **Run a transform on 1 domain**, where you
set them and press **Run**. A transform marked **key** needs an API key you
keep under [Secrets](/guide/secrets/).

You can also select several entities of the same type (Shift-click, or press
S and drag a box) and press **Transform** in the selection bar.

## Watch it run

The **Console** strip along the bottom of the sketch shows the latest log line
and how many scans are running. Open it (Cmd/Ctrl+D) for three tabs:

- **Log**: what every run did, line by line. Narrow it by level, or click a
  line to see its scan.
- **Scans**: each run with its status and outcome. A scan's buttons show only
  its log, or select on the canvas what it found.
- **Schedules**: flows that run again on this sketch every so often.

When a scan settles, the canvas reloads. The addresses `domain_to_ip` found
appear beside the domain, linked to it. Each new entity records where it came
from: open one and its panel says which scan found it and from what.

From here, run another transform on the new addresses, or chain several into a
[flow](/guide/flows/) to run them in one go.

## Export the sketch

Open the sketch menu (the button beside **Import**) and choose **Export**. Type
a passphrase twice to get an encrypted file, or leave it empty for plain JSON,
and press **Download**. The file holds the entities, the relationships and the
attached files, and imports into any gosint install. See
[Import and export](/guide/import-export/).

## Start from a sample

The release archive carries `examples/sketches/example-corp.json`, the external
exposure of a made-up company with the people behind it. Import it from the
**Investigations** page with **Import** to get a new investigation holding it,
or merge it into an open sketch through **Import**.
