---
title: Flows
description: "Chain transforms into a tree, run it on a selection, schedule it to run again and see what changed, and move flows as files."
weight: 50
---

A flow chains transforms. It starts from an input type, and each step runs
a transform on what the step before it **wrote**. A typical one: a domain,
its subdomains, their addresses, drop the CDN addresses, the open ports of
what is left.

A flow is a tree, not a line. Several steps can run after the same step,
each on its own branch. Flows are your own: other members do not see
them, and a flow runs with **your** secrets when you run it.

## The flows page

**Flows** in the top bar lists your flows. **New flow** opens the editor,
**Import** and **Export all** move flows as files (see below), and each
row has its own **Export**.

## The editor

The editor is a canvas with three parts.

- **The palette** on the left lists the transforms by category, the ones
  that take the selected step's output first. Click one to add it after the
  selected step, or drag it onto the canvas.
- **The canvas** in the middle draws the input and the steps, each as a
  node with its input type and output type. A step whose transform does not
  take what its parent gives is drawn red.
- **The panel** on the right edits what is selected.

With the input selected, the panel sets the **Starts from** type and has
**Add a step** and **Tidy layout**.

With a step selected, it shows **Transform, on** the type the step
receives, the transform's description, what it takes and gives, and the
secrets it needs. Only the transforms that fit are offered. Below come the
parameters, each with **default** as a choice, **Add a step after** and
**Remove**. Removing a step hands its children to its parent.

### Wiring

Drag from a node's right edge onto a step to make that step run after the
node. A step has one parent, so a new wire replaces the old one, and a wire
that would make a loop is refused. Select a wire and press Backspace to cut
it. A step with no wire says **Not connected** and the flow will not save
until you wire it.

The positions you give the nodes are saved with the flow.

### Checks

**Check** asks the server to verify the flow without saving it, **Save**
verifies and saves. Both refuse:

- a transform this server does not have
- a step whose parent does not exist, or a loop
- a step whose input type its parent does not produce
- a parameter value its transform refuses

## Running a flow

Select entities of the flow's input type and pick the flow from the
**Flows** tab of the right click menu, or from the run dialog's **Flows**
list. The assistant can run one too.

The whole flow is one scan. Each step gets the entities its parent's run
wrote, keeps those of its own input type (at most 500), and runs on them.
A step that fails is logged and the steps under it are skipped, the other
branches go on. In the **Scans** tab, **steps** shows what each step did,
and **found** selects everything the flow found.

## Schedules

A flow can run again on its own, so you see what appeared and what went
away since the last run. Open the console's **Schedules** tab and press
**Schedule a flow**:

- pick one of your flows
- pick how often: **every hour**, **every 6 hours**, **every 12 hours**,
  **every day** or **every week**
- pick what it runs on: **on every entity of the input type** (taken
  again at each run, up to 1000), or **on the selection** (those exact
  entities)

Each schedule shows when it last ran and a summary of that run:

- **new**: entities the run found for the first time
- **seen again**: entities it found again
- **missing**: entities the run before found and this one did not

**scan** selects what the last run found. **Run now** queues it at once,
**Pause** and **Resume** stop and restart it, **Delete** removes it.

A scheduled run is queued as the person who made the schedule and spends
their secrets. Only that person or a member who manages the investigation
can change, run or delete a schedule. A schedule stops by itself when its
flow is deleted or no longer passes the checks (the error is shown on the
row), and when its owner can no longer edit the investigation.

{{< callout type="tip" title="Watching for change" >}}
Schedule a flow from a domain to its subdomains and their open ports, on
every domain of the sketch, every day. **missing** then shows a host that
went away, **new** a host that appeared.
{{< /callout >}}

## Flows as files

**Export** on a row downloads one flow as JSON, **Export all** downloads
every flow of yours in one file. **Import** takes either kind of file and
creates the flows as yours. A flow that uses a transform this server does
not have, or whose name you already use, is skipped and named in a
message. The rest are imported.
