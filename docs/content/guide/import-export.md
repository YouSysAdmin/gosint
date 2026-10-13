---
title: Import and export
description: "Bring in lists, tables, nmap and nuclei reports and export files, and take sketches and investigations out as one file."
weight: 60
---

## Importing into a sketch

**Import** in the sketch header (or **Import entities** on the empty
canvas) opens the import dialog. It takes five kinds of input.

### A list of values

Type or paste one value per line and press **Recognize**. Each line's type
is recognized for you (an email, an address, a domain, a hash...), and a
table shows what was found. Untick the lines you do not want and press
**Import**. A value its type refuses is shown with the reason and cannot
be ticked.

### An export of a sketch

Choose a `.json` or `.gosint` export file. Its entities, relationships and
files are **merged** into this sketch. An entity that is already here is
updated, not duplicated. The sketch name in the file is ignored. An
encrypted export needs its passphrase in the field above.

### An nmap report

Run nmap with XML output (`nmap -oX scan.xml ...`) and choose the file.
Each host with an IP address becomes an `ip`, its **open** ports become
`port` entities linked with `HAS_PORT` (with the service and banner), and
the names nmap knows become domains. Hosts known by a MAC address alone are
skipped.

### A nuclei report

Run nuclei with `-jsonl` or `-json-export` and choose the file. Each result
becomes a **finding**, unverified, with the template as its rule and where
it matched as its location, under the website, domain or address it was
found on. Findings are flagged by severity (critical red, high orange,
medium yellow, low blue), and the CVEs a template names become `cve`
entities.

Import a newer report of the same targets and gosint compares them: every
finding gets a `last_seen` date, and a finding the new report no longer
has is marked `missing_since`. A finding you already triaged keeps the
flag you gave it.

### A table (CSV or XLSX)

Choose a CSV file (comma, semicolon or tab, detected) or an XLSX workbook
(its first sheet). The dialog shows the first rows and drafts an entity
block for each column whose type it recognizes.

- Each block is one entity per row: pick its type, then a column for each
  field. The primary field (marked `*`) is required. **Add a field...**
  maps more columns.
- **Add an entity** adds a block, **Remove** drops one.
- **Add a relationship** links two blocks with a label, for every row.

A row that leaves a block's primary field empty skips that entity. The
same value on two rows is one entity. A list field splits on semicolons,
pipes or line breaks, never on commas. Press **Import N rows**.

A table holds at most 20000 rows.

### After an import

The dialog says how many entities and relationships were imported and how
many were skipped, with the reason for each skipped one. Imported entities
record where they came from ("imported from a nuclei report by Jane").
Imports are not part of undo.

## Exporting

### A sketch

**Export** in the sketch menu downloads the sketch with its entities,
relationships and files as one file. With a **Passphrase** (typed twice)
the file is encrypted and opens only with it. Leave it empty for a plain
JSON file.

### An investigation

**Export** on the investigation page downloads the investigation with
every sketch, their files and the analyses, the same way.

### A selection

Right click a selection and choose **Export N entities as JSON**, or **Copy
N labels as text** for a plain list.

### Tables and pictures

- The **Table** and **Relationships** views have a **CSV** button with what
  they show. A value that is not text is written as JSON, and a cell that a
  spreadsheet would run as a formula is written as text.
- **Export the graph as PNG** in the sketch menu saves the canvas as drawn.
  It works only while no entity shows a picture loaded from another site.

## Bringing an export into another install

An export file moves between gosint installs. The other side needs no key
of yours, only the passphrase if you set one.

- On the **Investigations** page, **Import** turns an investigation export
  into a new investigation you own. A sketch export becomes an
  investigation with one sketch.
- On an investigation page, **Import sketch** turns a sketch export into a
  new sketch there, named as the file says.
- In a sketch, the import dialog **merges** an export into it (see above).

Entities of a [custom type](/extending/custom-types/) import only where an
administrator declared the same type.

{{< callout type="warning" title="Keep the passphrase" >}}
An encrypted export cannot be opened without its passphrase, by you or by
anyone else. Nothing on the server can recover it.
{{< /callout >}}

Flows and templates move as files of their own, see
[Flows](/guide/flows/#flows-as-files) and
[Transform templates](/extending/transform-templates/).
