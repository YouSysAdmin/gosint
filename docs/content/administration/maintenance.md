---
title: Maintenance
description: "The scheduled jobs, running one now, sealing a node, the health check and the release check."
weight: 100
---

## The jobs

gosint runs a few jobs on a schedule inside every process. **Every node of a
team install runs every job**, and each job is written so that two nodes
running it at once do no harm. All of them are skipped while the server is
sealed.

| Job | Schedule | What it does |
| --- | --- | --- |
| `blob-release` | every hour | Deletes the bytes of files deleted longer ago than **Keep deleted files** (see [File store](/administration/file-store/)), unless the content came back |
| `undo-journal` | every hour | Drops hand edits older than the same retention from the undo history, so what can be undone still has its files |
| `sessions` | every hour | Removes expired sign-in sessions |
| `password-resets` | every hour | Removes spent and expired password reset links |
| `schedules` | every minute | Runs the [scheduled flows](/guide/flows/) that are due, each one exactly once across the nodes |
| `settings` | every minute | Reads the settings saved on another node: mail, worker, tools, file store |
| `custom-types` | every minute | Reads the entity types declared on another node |

**Admin > Maintenance** lists the jobs of the node answering, with the
schedule, the last run (its duration and its error, if any) and the next run.
**Run now** runs one job on this node and waits for it. A failing job shows
its error in the list, the details are in the server log.

## Sealing this server

**Seal this server** at the top of the page forgets the master key on this
node. Nothing opens and running scans fail until someone enters the unseal key
again, and the console goes to the unseal page. Other nodes stay unsealed. See
[The vault](/administration/vault/#sealing-a-node).

## The health check

`GET /health` answers `200` with `{"status": "ok", "database": "sqlite"}` (or
`postgres`) when the database answers, and `503` otherwise. It needs no
session and works while the server is sealed, so a load balancer or a
container health check can use it. A sealed server is healthy: it is up and
waiting for its key.

## The release check

When an administrator opens the console, gosint asks GitHub whether a newer
**stable** release exists and, if so, shows **Update X.Y.Z** beside the
account menu, linking to the release notes. Drafts and pre-releases are never
offered.

- GitHub is asked only when an administrator opens the console, at most every
  12 hours (after a failure, an hour later), never in the background.
- A development build (version `devel`) asks nothing.
- Members see no badge.

Turn it off with `console.update_check: false` (or
`GOSINT_CONSOLE_UPDATE_CHECK=false`): then nothing is ever sent.

## Logs

The server logs to standard output as JSON by default. `logging.format: text`
is easier to read in a terminal, and `logging.output` takes a file path. See
the [configuration reference](/reference/configuration/#logging).
