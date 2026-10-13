---
title: Investigations
description: "Create a case, invite the people working on it, give each a role and move the whole thing between installs."
weight: 10
---

An investigation is one case. It holds the sketches (the graphs you work
on), the written analyses, and the people who may see them. Everything you
do in gosint happens inside an investigation.

## The investigations list

**Investigations** in the top bar lists every investigation you are a
member of. Search by name, narrow by status with the chips (each shows a
count), and switch between **Cards** and **List**. The layout you pick is
remembered in this browser. Archived investigations sink to the end.

Each card shows your role on it and when it was last updated.

**New investigation** asks for a name and an optional description. You
become its owner.

## The investigation page

Open an investigation to see:

- **Sketches**, as cards with their description. **+ New sketch** creates
  one, **Import** brings in a sketch from an export file.
- **Analyses**, the written accounts of the case. **+ New analysis**
  starts one (see [Analyses](/guide/analyses/)).
- **Members**, with their role.

The header holds the status select (**Active** or **Archived**), **Edit**
for the name and description, **Export**, **Import sketch**, **New
sketch**, and **Delete** for the owner.

## Roles

Every member has one role on the investigation. It decides what they can
do with it and with every sketch inside it.

| What | owner | admin | editor | viewer |
| --- | :---: | :---: | :---: | :---: |
| Open the investigation, its sketches and analyses | yes | yes | yes | yes |
| Chat with the assistant (read only for a viewer) | yes | yes | yes | yes |
| Create sketches and analyses | yes | yes | yes | |
| Edit the graph, run transforms and flows, import | yes | yes | yes | |
| Edit the name, description and status | yes | yes | | |
| Add and remove members, change roles, invite | yes | yes | | |
| Delete an analysis | yes | yes | | |
| Delete a sketch or the investigation | yes | | | |
| Hand ownership to someone else | yes | | | |

There is exactly one owner. Someone who is not a member sees nothing of
the investigation, not even that it exists: its address answers as if it
were not there.

## Sharing

**Share** under Members opens the share dialog. Type an email address and
pick a role:

- **viewer, reads**
- **editor, reads and changes sketches**
- **admin, also manages members**

People you already share an investigation with are suggested as you type.
Anyone else is added by their full address, so nobody can list the
accounts of an install by typing a few letters.

Change a member's role with the select beside their name, or **Remove**
them.

### Invitations by link

When the address has no account yet, gosint creates an invitation
instead. You get a link to hand over, and when the server can send mail
the invitation is mailed too. The link is shown once, so copy it before
you close the dialog.

The person opening the link sees which investigation they are invited to
and with which role. They sign in, or create an account with that address
(the invitation lets them register even when registration is closed), and
press **Join the investigation**. An invitation is valid for seven days and
works once.

Pending invitations are listed under the members. **Revoke** cancels one.

## Handing ownership over

The owner can press **Make owner** on another member. That person becomes
the owner and you become an admin on the investigation. An account that
owns an investigation cannot be deleted by an administrator until it hands
its investigations over.

## Status and archive

Set the status to **Archived** when a case is closed. Nothing is removed
or locked, the investigation just moves to the end of the list. A sketch
has the same status in its settings.

## Moving an investigation as one file

**Export** downloads the whole investigation, with every sketch, its files
and the analyses, as one file. With a passphrase the file is encrypted and
opens only with it. Without one it is plain JSON.

On the investigations list, **Import** turns such a file into a new
investigation that you own. An export of a single sketch works too and
becomes an investigation with one sketch in it. See
[Import and export](/guide/import-export/) for the details.

{{< callout type="warning" title="A plain export is not encrypted" >}}
Inside gosint everything is encrypted at rest. An export downloaded
without a passphrase is readable by anyone who gets the file. Use a
passphrase for anything you send or store elsewhere.
{{< /callout >}}
