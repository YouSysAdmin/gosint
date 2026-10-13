---
title: Custom entity types
description: "Entity types an administrator declares as data, beside the built-in ones."
weight: 30
---

The built-in entity types cover the usual ground of an investigation,
see [Entity types](/reference/entity-types/). When your work keeps
coming back to something they do not name (a badge, a case number, a
vessel, a forum thread), an administrator can declare it as a type of
its own. It then sits beside the built-in ones for every account: in
the palette, in the add and edit forms, in the table import, in
filters, and as the input or output of a
[template](/extending/transform-templates/) or a
[script](/extending/transform-files/).

{{< callout type="tip" title="One entity, or every entity of a kind" >}}
A field that only one entity needs is a custom field on that entity,
added from its panel, with no type to declare. Declare a type when the
same shape repeats.
{{< /callout >}}

## Declaring a type

Administration, **Entity types**, **Declare a type**. Only an
administrator sees the page.

| Field | What it is |
| --- | --- |
| **Name** | The type's key: lowercase letters, digits and `_`, starting with a letter, up to 41 characters. It cannot be the name of a built-in type, and it is **fixed once made**. Exports, flows and templates name the type by it. |
| **Title** | What people read, such as "Vessel". Required. |
| **Category** | The palette group it is listed under: Global, Identities & Entities, Organization, Communication & Contact, Network, Security & Access, Files & Documents, Financial Data, Leaks or Crypto. |
| **Icon** | One of the bundled icons, picked from a list. Drawn in the palette and on the node. |
| **Colour** | The node's colour, as `#RRGGBB`. |
| **Description** | A line about the type. |

### Fields

A type has one to 64 fields. Each field has:

| Setting | What it is |
| --- | --- |
| **Name** | The property key: lowercase letters, digits and `_`, starting with a letter. `label` and `nodelabel` are taken. Each name once. |
| **Title** | What the form shows. Without one, the name with spaces for underscores. |
| **Kind** | Text, Whole number, Number, Yes/no, Date, URL, Email or List. |
| **Description** | Shown under the input when the entity is edited. |
| **Options** | For a Text field only: one value per line, and the form offers them as a select. Empty leaves it free text. |
| **Required** | An entity cannot be saved without it. |
| **Long text** | For a Text field: a text area instead of one line. |

Every value is checked by its kind when an entity is saved, by hand, by
an import or by a transform. A number that is not a number, or an
address that is not an email, is refused with the field named.

### The primary field

Exactly **one** field is the primary one, chosen with the radio button
on its card. It must be Text, URL or Email, and it is always required.
It is:

- **the entity's name**, the label drawn on the canvas and listed
  everywhere
- **the entity's identity**: within a sketch, two entities of the type
  with the same primary value are one entity, so adding it again, or a
  transform finding it again, updates it instead of making a second one
- **what a typed value fills**: when someone adds an entity of the type
  by typing one line, or a transform writes one by label, the value goes
  into the primary field

Pick the field that tells two of them apart: a hull number rather than
a vessel name if names repeat.

## What changes when you save

A type saved on a node applies at once on that node. Every other node
of a team install learns it within a minute. The palette in an open
console picks it up on its next load.

Editing a type later changes its title, category, icon, colour and
fields. The name stays. Entities already in the sketches keep the
properties they have: a field you remove is not deleted from them, and
a field you add starts empty.

## What a custom type does not do

- **It is never detected from text.** Pasting or importing plain text
  guesses among the built-in types only, and the table import does not
  guess a column as a custom type. Pick the type yourself: in the
  palette, in the add form, or in the table import's mapping.
- **It has no rules of its own** beyond the field kinds. The built-in
  types clean values up (a domain is lowercased, a card number checked,
  a wallet's currency guessed). A custom type stores what each field's
  kind accepts, and its label is its primary field as given.
- **No field holds another entity.** Fields are values, a relationship
  to another entity is an edge on the canvas.
- **Built-in transforms do not take it.** They are written for the
  built-in types. Write a [template](/extending/transform-templates/)
  or a [script](/extending/transform-files/) that takes or produces it.

## Deleting a type

**Delete** removes a type only while no entity of it exists in any
sketch on the install. Otherwise the server answers how many entities
use it and keeps the type. Delete those entities first.

## Exports between installs

An export names each entity's type by its name. When a sketch or an
investigation holding custom types is imported on another install, its
entities of a type that install does not have are refused and listed
in the import's answer, and the relationships touching them are
skipped. Declare the type there first, **with the same name** and the
fields you need, then import.
