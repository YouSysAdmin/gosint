---
title: Entities
description: "Typed entities: adding them, editing their fields, custom fields, style, relationships, merging, deleting, where they came from, files and notes."
weight: 30
---

An entity is one thing you found: a domain, an IP address, a person, an
organization, a wallet, a credential, a finding. Every entity has a
**type**, and the type decides its fields, its icon and colour, and what
makes it unique.

## Types and identity

Each type has one **primary field**: the domain name of a domain, the
address of an email, the number of an AS. That value is the entity's
identity in the sketch. Adding the same value again, by hand or through a
transform, updates the entity that is already there instead of making a
second one. New fields are merged in, nothing an earlier find recorded is
erased.

The label you see on the canvas is computed from the fields, so it follows
when a transform fills in more.

The built-in types are listed in [Entity types](/reference/entity-types/).
An administrator can declare more, see
[Custom types](/extending/custom-types/).

### Detection from text

Wherever you type or paste a plain value, gosint recognizes its type: an
address with an `@` is an email, `8.8.8.8` an IP address, `AKIA...` an API
key, `git@github.com:owner/name.git` a repository, two or three words a
person. A value nothing matches becomes a **phrase**. Detection runs in the
quick add box, in the text import, and in the table import's column
guesses.

## Adding an entity

- **Add** in the header opens the add dialog. Pick a type, fill the fields
  and press **Add**. The required fields are always shown, the rest sit
  behind **More fields**.
- **Double click** the empty canvas for the quick add box at that spot.
  Type a value, its type is recognized as you type, Enter adds it.
- In the left panel's **Add** tab, click a type card to open the dialog with
  that type, or drag the card onto the canvas to place the entity where you
  drop it.

With one entity selected, the add dialog offers to link the new one to it:
tick **Link it to**, pick the **Direction** and the **Relationship**, and
press **Add and link**. **Add a related entity** in an entity's right click
menu does the same.

## The details panel

Click an entity to open its panel on the right. The name at the top renames
the entity on click. Below it:

- **Transform** to run a transform on it, **Link to...** to draw a
  relationship to another entity, **Clone**, **Edit**.
- The **Properties** tab: the fields as a table. Long values fold behind
  **Show more**, a secret field (a password, a key) is masked until
  **Show**, URLs are links, and every value has a copy button.
- **Added** and **Last seen** (see [Provenance](#provenance)).
- **Neighbours**: a small map of the linked entities, each drawn in its
  direction on the canvas.
- **Also in**: other sketches you can open that hold the same entity.
- **Relationships**, grouped by direction and label, with **select all**
  on each group and a button to remove one relationship.
- **Notes**.
- The **Style** and **Files** tabs.

## Editing fields

**Edit** turns the properties into the same form as the add dialog. Only
the fields you change are sent. Clearing a field removes it.

A list field (emails, aliases, phones) takes one value per line. A field
that names another entity, such as a person's current address, is typed as
one line. A long field, such as a description or a bio, is a text area.

Renaming the entity changes its primary field, and with it its identity.

### Custom fields

Every entity can carry fields its type does not have. In the edit form,
under **Custom fields**, press **Add a field**, give it a name (starting with a letter, then letters,
digits, spaces, `-` and `_`) and pick what the value is: **Text**,
**Number**, **Yes/no** or **Date** (stored as YYYY-MM-DD). A value that
does not fit its kind blocks the save and says which field. Clear a custom
field to remove it.

Custom fields show in the filters like any other property.

## Style

The **Style** tab sets how the entity is drawn: its **Size** (or **Auto**,
growing with its relationships), its **Colour**, its **Shape** (circle,
square, hexagon, triangle) and an **Image URL** drawn inside the node.

A **flag** is a coloured mark (red, orange, yellow, green, blue) set from
the right click menu. Some transforms set one too, for example red for an
address with a bad reputation. The left panel narrows its list by flag, and
`flag` is one of the properties the filters offer.

## Cloning

**Clone** (in the panel or the right click menu) opens the add dialog
prefilled from an entity: the same type, the same custom field names with
empty values, the same colour, icon, shape and size. Values, notes, files
and relationships are not copied. The clone lands beside the original.
This is the quick way to add ten entities that share a set of custom
fields.

## Relationships

A relationship joins two entities in one direction and carries a label,
written in capitals like `HAS_EMAIL` or `RESOLVES_TO`. The same label
between the same two entities exists once.

- **Link to...** in the panel, **Alt drag** on the canvas, or **Link** with
  two entities selected opens the link dialog. Swap the direction with the
  arrow, pick the label from the suggestions or type one.
- Click a wire to select the relationship, see where it came from, edit its
  label or delete it. The right click menu on a wire does the same.

## Merging

Select several entities of one type and press **Merge**. They become the
first one you selected. Every relationship of the others moves to it with
its own label, relationships among the merged entities are dropped, and
the kept entity keeps its type.

## Deleting

**Delete** (or the Delete key) removes the selection and the relationships
touching it. Before it does, gosint looks for what was found through what
you delete: the entities that hang only from it and did not exist before
it. When there are some, the dialog lists them behind **Also delete the N
entities found through it**, unticked by default. An entity also connected
to the rest of the sketch always stays.

So deleting a domain you added by mistake can take its subdomains and
their addresses with it in one step, without touching anything you had
before.

## Provenance

Every entity and relationship records where it came from, once, when it
first appears. The panel shows it under **Added**:

- "found by domain_to_subdomains from example.com", with a **scan** link to
  the run that found it
- "added by Jane" for a hand edit
- "imported from a nuclei report by Jane", "imported from a table (CSV or
  XLSX) by Jane", "imported from an export file by Jane"
- "added from the assistant by Jane"

**Last seen** shows the latest run that found the entity again. That is
how a scheduled flow can tell what went missing.

## Files

The **Files** tab attaches files to an entity: screenshots, documents,
mails. Drop them on the tab or press **Choose files**. The largest file
allowed is an administrator's setting (32 MB unless changed).

Click a file to open it in the viewer:

- pictures, PDF, audio and video play in place
- text files (logs, JSON, YAML, CSV, code) are printed, up to 2 MB
- an `.eml` file opens as a message, with **HTML**, **Text**, **Headers**
  (in message order, the Received chain included), **Attachments** and
  **Raw** tabs. The HTML is shown sandboxed, with remote images held back
  until you press **Load images**.

Anything else downloads.

At upload gosint reads what a file says about itself: a photo's EXIF (the
camera, when it was taken, the GPS position), a PDF's or an Office
document's author, title, dates and company. Those values fill the
entity's **empty** fields, never one you set. The file's SHA-256 fills a
`hash` field when the type has one. The `files_to_entities` transform turns
the same metadata into entities: a location from GPS, the author as a
person, the camera as a device.

**Use as the picture** on an image file draws it inside the node. Removing
a file or the entity does not destroy the bytes at once, so an undo brings
them back for as long as the server keeps them.

## Notes

The **Notes** box at the bottom of the Properties tab is free text about
this entity, saved on its own as you type. For a longer written account
with references to several entities, use an [analysis](/guide/analyses/).
