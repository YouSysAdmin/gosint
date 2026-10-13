---
title: Mail
description: "The SMTP relay for password reset and invitation links."
weight: 50
---

gosint sends two kinds of message: **password reset links** and
**invitations** to an investigation. Both are links built from
`server.public_url`, so set it first.

Mail is optional. Without a relay nothing is sent, the reset and invitation
links are shown to the administrator or the person sharing, and they hand
them over themselves. A "Forgot password" request gives the same answer
either way, so it never tells anyone whether an address has an account.

## Setting the relay

**Admin > Mail**:

| Field | Default | Meaning |
| --- | --- | --- |
| **Host** | empty | The SMTP server. Empty turns mail off |
| **Port** | 587 | 465 for implicit TLS |
| **Sender** | | The From address, a display name allowed: `Gosint <gosint@example.com>` |
| **Username**, **Password** | | The relay's credentials. The password is never shown again, leave it empty to keep the stored one |
| **STARTTLS** | on | On, the connection upgrades with STARTTLS and refuses a relay that does not offer it. Off, it connects with implicit TLS, port 465 |

Unchecking STARTTLS moves the port from 587 to 465 and back, when it still
holds the other default.

The settings are sealed in the database with everything else. A save applies
at once on the node that made it, and every other node picks it up within a
minute.

## Testing

**Send a test message** sends a message to your own address through the
**saved** relay, so save first. An error from the relay is shown as it came.
