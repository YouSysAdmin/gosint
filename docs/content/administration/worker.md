---
title: Worker
description: "How transforms and flows run: the queue, concurrency, leases and retries."
weight: 60
---

A transform run, a flow run and a scheduled flow are each a **scan**. Scans
wait in a queue that is a table of the database, and a **worker** in every
gosint process claims and runs them. There is no Redis or other broker to run.

The worker starts only once the server is unsealed and the settings are read,
and it claims nothing while the server is sealed.

## The settings

**Admin > Worker** applies to every node, and a change applies without a
restart.

| Field | Default | Range | Meaning |
| --- | --- | --- | --- |
| **Run transforms and flows** | on | | Off, launched scans wait in the queue until it is on again |
| **Cache directory** | see below | | Where transforms keep downloaded reference data, such as the CDN address ranges. An absolute path |
| **Concurrency** | 4 | 1 to 64 | Scans running at once on each node |
| **Queue check, seconds** | 2 | 1 to 300 | How often a node looks for scans queued on another node |
| **Lease, seconds** | 120 | 10 to 3600 | A scan whose node stopped renewing it runs again after this |
| **Attempts** | 3 | 1 to 20 | How often a scan whose node died is tried before it fails |

The default cache directory is `cache` beside the SQLite database, or the
user cache directory of the server account on PostgreSQL.

## How a scan runs

- **Launch.** A scan queued on a node wakes that node's worker at once. Other
  nodes find it on their next queue check.
- **Claim.** Each scan is taken by exactly one worker, even with several
  nodes polling the same table.
- **Lease.** The worker holding a scan renews its lease every third of the
  lease time. **Cancel** in the Scans drawer is seen at the next renewal and
  stops the run.
- **A node dies.** Its leases run out and any worker puts the scans back in
  the queue, up to **Attempts** tries, then fails them.
- **A node stops.** On a clean shutdown the worker hands its running scans
  straight back to the queue for another node.

Logs of a run are written to the database and streamed live to everyone who
has the sketch open, through every node.

## Sizing

Most transforms wait on the network, so concurrency is about how many third
party requests you want in flight, not CPU. Raise it for a busy team install,
lower it if a source starts rate limiting. A crawler or an nmap scan holds a
slot for its whole run.

## Turning it off

Unchecking **Run transforms and flows** keeps the console fully usable for
editing graphs while nothing runs. Use it before maintenance on a third party
key, or on a node that should serve the console only, keeping in mind that
the switch is shared by every node.
