---
title: File store
description: "Where files attached to entities are kept: a directory, an S3 bucket or the database."
weight: 70
---

Files attached to entities (pictures, documents, mail, reports) are sealed
before they are stored, wherever that is. The record of a file (its name,
type, size) lives in the database, the bytes in the **file store**.

Two records with the same content share one stored copy.

## The settings

**Admin > Files**:

| Field | Default | Meaning |
| --- | --- | --- |
| **Largest file, MB** | 32 | The biggest upload accepted, 1 to 4096 |
| **Keep deleted files, hours** | 168 (7 days) | How long the bytes of a deleted file stay, so an undo brings it back. The undo history is kept as long |
| **Store** | a directory on SQLite, the database on PostgreSQL | Where the bytes go |

### A directory on the server

**Directory** is an absolute path. Empty uses `files` beside the SQLite
database. On PostgreSQL there is no default: give a directory **every node
shares**, such as an NFS mount, or pick another store.

### An S3-compatible bucket

| Field | Meaning |
| --- | --- |
| **Endpoint** | Empty for AWS, the URL of anything else (MinIO, Ceph, R2, Backblaze) |
| **Region** | The bucket's region |
| **Bucket** | Required |
| **Prefix** | Put in front of every key, to share a bucket |
| **Access key**, **Secret key** | Empty uses the server's own credentials (environment, instance role). The secret is never shown again |
| **Path-style addressing** | For MinIO and most self-hosted stores |

### The database

The bytes go into a table of the database itself. It suits a PostgreSQL team
install with no shared directory and no bucket, at the price of a larger
database and larger backups.

## Saving

A save is refused unless the store answers, so a wrong path or key is caught
before anything is written.

{{< callout type="warning" title="Changing the store moves no file" >}}
The files already attached stay in the old store and cannot be opened from
gosint until you copy them into the new one yourself. The console asks before
it saves such a change. Copy the stored objects as they are, under the same
names: they are already sealed, and gosint finds them by name.
{{< /callout >}}

## Deleted files

Deleting an entity, clearing a sketch or deleting a file only **marks** its
bytes. The `blob-release` job deletes them once **Keep deleted files** has
passed, unless an undo or another entity brought the content back by then.
See [Maintenance](/administration/maintenance/).

## What a file store holds

Every stored object is encrypted (AES-256-GCM in 64 KiB chunks) under a key
derived from the master key and the file's digest. A bucket or a directory
leaked on its own reveals neither the content nor which known file it is.
