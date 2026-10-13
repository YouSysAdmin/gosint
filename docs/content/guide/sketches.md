---
title: Sketches and the canvas
description: "The sketch page: the graph canvas, selecting and linking, layouts, collections, filters, the views, undo and the console drawer."
weight: 20
---

A sketch is one graph inside an investigation: a canvas of entities and
the relationships between them, with its own scans, log, chats and
schedules. Open one from its investigation page.

## The page at a glance

- **The header** reads investigation / sketch. Click the sketch name to
  switch to another sketch of the same investigation. Beside it you see how
  many entities and relationships the sketch holds, and the **Find an
  entity** field.
- **Add**, **Import** and a menu with **Sketch settings**, **Shortcuts**,
  **Export**, **Export the graph as PNG**, **Clear the sketch** and
  **Delete the sketch** (owner only).
- **Notes** and **Chat** open the [notes](/guide/analyses/) and the
  [assistant](/guide/assistant/) beside the graph.
- The view switch: **Graph**, **Table**, **Relationships**, **Map**.
- **The left panel** (<kbd>Cmd/Ctrl</kbd>+<kbd>B</kbd>) with the entity list
  and the type cards.
- **The right panel** with the selected entity or relationship. It lies
  over the graph, so opening it never moves the canvas.
- **The console** at the bottom (<kbd>Cmd/Ctrl</kbd>+<kbd>D</kbd>) with the
  **Log**, **Scans** and **Schedules** tabs.

## Selecting

- **Click** an entity to select it and open its details.
- **Shift click** or **Cmd/Ctrl click** adds to the selection or takes an
  entity out of it.
- **Hold S and drag**, or arm the box tool in the toolbar, to select with a
  box.
- **Cmd/Ctrl+A** selects every entity shown.
- **Escape** or a click on the empty canvas clears the selection.

With several entities selected a bar shows the count and the actions that
fit: **Transform** (when they share a type), **Link** (exactly two),
**Merge** (several of one type) and **Delete**.

Hovering an entity lights its neighbours and dims the rest. Relationship
labels are drawn only on lit wires, so a hub does not flood the canvas with
the same word.

## Adding and linking

- **Double click** the empty canvas to add an entity right there. Type a
  value and its type is recognized as you type.
- **Drag a type card** from the left panel's **Add** tab onto the canvas.
- **Alt drag** from one entity to another to link them. The link dialog
  asks for the relationship label.

See [Entities](/guide/entities/) for the add dialog and the details panel.

## Right click menus

A right click never changes the selection. The menu acts on what it was
opened on:

- **An entity**: copy the label, open its details, **Add a related
  entity**, **Link to...**, **Clone**, **Ask the assistant**, a **Flag**
  colour, the **Select** row, **Delete**, and the **Transforms** and
  **Flows** that take its type, with a search field. A gear marks a
  transform that asks for parameters and **key** one that needs a secret.
- **A selection**: the same for every selected entity, plus **Copy N labels
  as text** and **Export N entities as JSON**.
- **A relationship**: edit its label, or **Delete the relationship**.
- **The empty canvas**: **Select everything shown**, **Invert the
  selection**, **Add an entity here**, **Import entities**.

### Selection tools

The **Select** row on an entity or a selection replaces the selection
with:

- **neighbours**: every entity one relationship away
- **parents**: what the relationships come from
- **children**: what the relationships lead to
- **connected**: the whole piece of the graph joined to it
- **same type**: every entity of that type
- **path between** (exactly two selected): the fewest relationships joining
  them. A message says how many hops, or that nothing joins them.

A wire with both ends selected is lit, so a path reads on the canvas.

## The toolbar

The strip of icons on the top left of the graph:

- **Zoom in**, **Zoom out**, **Fit everything**, **Zoom to the selection**
- **Select with a box (hold S)**
- **Hide entities by a rule** (the filters)
- **Collections** (fold large groups, see below)
- **Load remote images**
- **Undo** and **Redo**, each with the edit it would revert as its tooltip
- **Force layout** and **Hierarchy layout**

## Layouts

Entities stay where you drag them. Two layouts rearrange the whole graph,
each behind a confirmation because the positions you set are lost:

- **Force layout** lets the simulation spread the graph out, then pins
  every entity where it settles.
- **Hierarchy layout** draws a tree from each piece's root, with a parent's
  leaves packed in a grid under it.

New entities that a transform brings in start beside an entity they are
linked to and settle there, while everything already placed stays put.

## Collections

When twelve or more entities of one type hang from one parent by the same
relationship, and nothing else links to them, they fold into one node drawn
as a stack, named like **24 Website**. A click selects its members, a right
click opens the selection menu on them, and a double click unfolds it.

A collection exists only in the view. It is never stored. Turn folding off
with the **Collections** button in the toolbar, which is remembered in this
browser.

## Filters and the legend

**Hide entities by a rule** opens the filters. A rule names a property, an
operator (**is**, **is not**, **contains**, **does not contain**,
**starts with**, **ends with**, **is more than**, **is less than**, **is
set**, **is empty**) and a value. Every property found in the sketch is
offered, custom fields and the flag included. Numbers compare as numbers,
and dates compare in order too. The rules are kept per sketch in this
browser, and the button shows how many entities they hide.

The legend on the graph lists the types present with a count. Click a
type to hide it without touching the layout, and click it again to bring it
back. **Alt click** (or Cmd/Ctrl click) shows only that type, and the same
again shows every type.

## Views

- **Graph**: the canvas.
- **Table**: every entity with its type and properties, a search over the
  properties, a type filter and a **CSV** download.
- **Relationships**: every relationship with both ends, a label filter and
  a **CSV** download. Click an end to jump to it.
- **Map**: the entities that have numeric `latitude` and `longitude`,
  clustered. Use `ip_to_infos` to place IP addresses and
  `location_to_coordinates` to place postal addresses.

The filter rules apply to the Graph and the Table.

## Live edits

When a teammate edits the same sketch, their change shows up for you
within a moment, and the header reads "Jane just edited" for a few seconds.
Your own edits are never echoed back.

## Undo and redo

**Cmd/Ctrl+Z** undoes your last hand edit of the graph and
**Cmd/Ctrl+Shift+Z** (or **Ctrl+Y**) redoes it. Hand edits are adding,
editing, deleting, clearing, merging and linking. Each account has its own
history per sketch, the last 50 edits.

A transform run, an import and an applied assistant proposal are not
undone this way. To take back a run, use **found** on its scan to select
what it found and delete that. Files deleted with an entity come back with
the undo, as long as the server still keeps them (a week by default).

{{< callout type="note" title="Undo after someone else's edit" >}}
An undo restores the entities as they were before your edit, even if a
teammate changed them since.
{{< /callout >}}

## The console drawer

The bar at the bottom shows the last log line and how many scans are
running. Open it with a click or **Cmd/Ctrl+D**.

- **Log**: every line the scans of this sketch wrote, live. Narrow it by
  level, click a line to jump to its scan.
- **Scans**: each run with its status (pending, running, completed, failed,
  cancelled), what it produced, **log** to show only its lines, **found** to
  select what it found, **steps** for a flow, and **Cancel** while it runs.
  See [Transforms](/guide/transforms/).
- **Schedules**: flows that run again on their own. See
  [Flows](/guide/flows/#schedules).

## Remote images

An entity can show a picture: a file you attached, or an avatar or logo
URL it carries. Pictures from this server always show. A picture hosted
on another site is loaded only after you turn on **Load remote images** in
the toolbar, because every fetch tells that site who opened the sketch. The
choice is kept per sketch in this browser.

## Search across sketches

**Cmd/Ctrl+J** opens the command palette anywhere in the console. It jumps
to an investigation, a sketch or a page, and from two characters on it also
finds entities by a piece of their label in every investigation you are a
member of. Pick a hit to open its sketch with the entity in focus.

The entity panel also shows **Also in**: the other sketches that hold the
same entity.

## Sketch settings

**Cmd/Ctrl+G** or **Sketch settings** in the menu edits the title, the
description and the status (**Active** or **Archived**). An archived sketch
is kept but sinks in the lists.

**Clear the sketch** removes every entity and relationship but keeps the
scans and the log. **Delete the sketch** removes it for good and is the
owner's alone.

All keys are listed in [Keyboard shortcuts](/reference/keyboard/).
