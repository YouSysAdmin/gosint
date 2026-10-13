---
title: Security model
description: "What is encrypted at rest, what stays in clear, how sessions and requests are guarded, and the limits that remain."
weight: 120
---

This page is for the person running the server: what gosint protects, how,
and what it leaves to you.

## Encrypted at rest

The database without the unseal key is meant to be a useless set of bytes.
Every value below is sealed with AES-256-GCM under a key derived from the
master key, and bound to its table, column and row, so a value copied into
another row does not open:

- **The graph**: every entity's label, properties, metadata and picture, the
  entity type, every relationship's label and properties, the undo history
- **The work around it**: investigation names and descriptions, sketch titles
  and descriptions, analyses, chats and their questions, scan inputs, results
  and errors, log lines
- **Files**: the record (name, type) and the bytes themselves, wherever the
  file store keeps them
- **Accounts**: email addresses, names, avatars, password hashes, the
  authenticator secret, session details, passkey names, invitation addresses
- **Configuration made in the console**: identity providers and their client
  secrets, the mail relay, the file store keys, model API keys, the worker and
  tool settings
- **What people build**: flows, transform templates, secret names and values,
  custom entity types, schedules
- **Events between nodes** over PostgreSQL `NOTIFY`

The master key itself is stored only wrapped under the unseal key (Argon2id,
then AES-256-GCM). See [The vault](/administration/vault/).

## Keyed indexes

A sealed value cannot be compared in SQL. Where gosint must find a row by
value (an account by email, an entity by type and key, a flow by name), it
stores a **keyed hash** (HMAC under a key derived from the master key) next to
the sealed value. The hash tells equal values apart without saying what they
are: someone with the database sees that two entities share a type, not which
type. Searching entities by a piece of their label works the same way, with
keyed hashes of short pieces, kept per sketch.

## Left in clear on purpose

Ids and foreign keys, times, counters, flags, roles, scan statuses, log
levels, sizes, canvas positions, an entity's drawing style (colour, icon,
shape, flag), token and code hashes, and passkey public keys. These describe
the shape of the data (how many sketches, how big, how active) but none of
its content.

An export downloaded **without** a passphrase is plain JSON, by the person's
choice.

## Sessions and cookies

- A session is a random token in the `gosint_session` cookie, HttpOnly and
  SameSite=Lax. Only its SHA-256 is stored, so a database read does not hand
  out sessions.
- Cookies are Secure when the listener does TLS or `server.public_url` is
  `https`.
- Short state (the second factor step, a passkey ceremony) travels in cookies
  sealed by the vault, with their expiry inside.
- Changing a password, a reset, turning 2FA on, and the administrator's
  actions end the account's other sessions.

## Requests to the server

- **Cross-site requests**: every request that changes something is refused
  unless its `Origin` or `Sec-Fetch-Site` shows it came from the console
  itself.
- **Bodies**: a request without a live session may send 1 MB at most. Only a
  signed-in request is allowed the larger upload and import limits, so nobody
  unauthenticated can make the server hold a large body.
- **Headers**: every API answer is `Cache-Control: no-store`, the console
  carries a Content-Security-Policy that runs scripts from its own build only,
  framing is denied, and `Permissions-Policy` turns off camera, microphone,
  geolocation, payment and USB. HSTS goes out when `public_url` is `https`.
- **Sign-in**: rate limited per address, slow refusals, lockouts after five
  wrong passwords or codes. See [Accounts](/administration/accounts/#signing-in-limits-and-lockouts).
- **Roles**: a person who is not a member of an investigation gets the same
  404 for it as for an id that does not exist, so ids cannot be probed.

## Requests from the server

Transforms, the assistant and identity providers make outbound requests. The
ones aimed at something an entity or a person names are guarded:

- loopback, private, link-local, CGNAT and other non-public ranges are refused
  at connect time, including after a redirect or a DNS answer pointing inward
- cloud metadata addresses are refused even when a lab switch allows private
  ranges
- a URL given as a transform parameter goes through the same guard

The switches that lift the private range refusal for a lab are per tool, see
[Tools](/administration/tools/). An identity provider has its own switch, see
[Single sign-on](/administration/sign-in/#outbound-requests).

## Serving files

Attached files are served through the server, behind the sketch's read access,
never from a public URL:

- only images, PDF, audio, video and plain text open in the browser, anything
  else (HTML, SVG, scripts) downloads
- the type is read from the name and the bytes, never from what the uploader
  claimed, and a mail part's type is sniffed too
- every served file but a PDF carries a sandboxing Content-Security-Policy
- an `.eml` message's HTML is shown in a sandboxed frame with remote images
  held back until the reader asks for them

## Scripts of transform files

On Linux with Landlock a transform file's script is confined: it reads the
system, its own directory and the paths an administrator adds, writes only a
scratch directory of its own, and never reaches the data directory, the
config, the unseal key file or the server's environment through `/proc`. See
[the sandbox](/extending/transform-files/#the-sandbox). On macOS and Windows
there is no sandbox, a script can read whatever the server's user can.

## What remains for you

These are known and decided, so plan around them:

- **Write access to the database is account takeover.** Roles, the
  administrator flag, linked identities and passkey credentials are not
  signed. Someone who can write to the database can make themselves an
  administrator. Protect the database credentials like the server itself.
- **An unsealed server holds the master key in memory.** Root on that machine
  reads everything.
- **An unattended unseal** (`vault.unseal_key_file`, `GOSINT_VAULT_UNSEAL_KEY`)
  puts the key beside the data.
- **Sealing is per node.** Seal this server on one node leaves the others
  open.
- **The assistant** streams on a detached 10 minute context, with no cap per
  account on how many answers run at once.
- **Uploads** are held in memory a few times over while they are processed.
  Keep **Largest file** reasonable.
- **The unseal key** needs only 12 characters. Choose a long one.
- **Scripts outside Linux** run unconfined, and a confined script's network is
  not limited.
- **Remote language models** receive what is selected in a sketch (secret
  fields excepted). Offer only models you trust with your investigations.
