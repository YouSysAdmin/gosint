---
title: Accounts and access
description: "The first administrator, registration, the Accounts page, invitations, and the limits on signing in."
weight: 30
---

## The first account is the administrator

A fresh install has no account. Once the server is unsealed, the sign-in page
sends you to **Set up gosint**, which creates the first account. That account
is an **administrator**, whatever the configuration says. It sees **Admin**
in the top bar, which holds every page of this section.

Registration asks for the password twice. A password is 8 characters at
least and at most 72 bytes (bcrypt reads no further).

## Registration is closed by default

With `auth.open_registration: false` (the default), only that first account
can register. Everyone else gets an account in one of three ways:

- an administrator creates it on the **Accounts** page
- a member of an investigation invites them by link (see below)
- an identity provider creates it on their first sign-in, when the provider
  allows it (see [Single sign-on](/administration/sign-in/))

Set `auth.open_registration: true` to let anyone who reaches the console sign
up. Accounts made that way are members, never administrators.

## The Accounts page

**Admin > Accounts** lists every account with a search by email, its role
(administrator or member), how it signs in (password or identity provider,
with 2FA and the number of passkeys), and when it was made.

**Add an account** takes an email, an optional password and the
**Administrator** box. Without a password, hand the person a reset link from
the account's **Manage** dialog.

**Manage** opens one account:

| Control | What it does |
| --- | --- |
| **Administrator** | Grants or removes administration |
| **Disabled, cannot sign in** | Blocks the account, ends its sessions, and its scheduled flows stop running |
| **Password**, **Set** | Sets a new password and signs the account out everywhere |
| **Reset link** | Makes a single-use link to choose a password, valid 30 minutes. It is mailed too when [mail](/administration/mail/) is configured, otherwise copy it and hand it over |
| **Remove 2FA** | Turns off the account's authenticator codes, deletes its recovery codes and ends its sessions |
| **Remove passkeys** | Deletes every passkey of the account and ends its sessions |
| **Sign out everywhere** | Ends every session of the account |
| **Owns** | The investigations the account owns |
| **Delete account** | Deletes the account with its flows, chats, secrets and templates |

An account signing in through an identity provider has no password controls.

### What an administrator cannot do to themselves

An administrator cannot change their own flags, remove their own 2FA or
passkeys, set their own password here, make a reset link for themselves or
delete themselves. Those go through the profile, which asks the current
password, or through another administrator. A stolen administrator session
therefore cannot weaken or take over the account it was stolen from.

### The last administrator stays

The server refuses to demote, disable or delete the last enabled
administrator. Two administrators demoting each other at the same moment
still leave one. Make another administrator first.

### Owners hand their investigations over first

An account that owns an investigation cannot be deleted, and **Delete
account** stays greyed out with the list under **Owns**. The owner opens each
investigation and uses **Make owner** on another member in the members list.
The previous owner stays on it as an admin. Then delete the account.

## Invitations by link

Sharing an investigation with an address that has no account makes an
**invitation** instead. The person sharing it gets a link once, it is mailed
when mail is configured, and it is valid for 7 days. The link admits a
registration for that one address even while `open_registration` is off, and
lands the new account on the investigation. A person who has an account signs
in and accepts with the same email address.

Pending invitations are listed under the members of the investigation, where
they can be revoked. The link itself is never shown again.

## Roles in an investigation

Being an administrator is about accounts and settings. Access to the work is
per investigation, by role: owner, admin, editor, viewer. See
[Investigations](/guide/investigations/).

## Signing in: limits and lockouts

| What | Limit |
| --- | --- |
| Requests to sign in, register, unseal and reset a password | `auth.rate_limit_per_minute` per address (10 by default), IPv6 counted per /64 |
| Wrong passwords | Five in a row lock the account for 15 minutes |
| Wrong authenticator codes | Five lock the second factor for 15 minutes |
| A refused sign-in | Answered no sooner than 750 ms |
| Password reset requests | 3 an hour per address, 15 per account, the same answer whether the address exists or not |

Behind a reverse proxy the limit only sees the real client address when the
proxy is listed in `server.trusted_proxies`. See
[Deployment](/administration/deployment/).

## Sessions

A sign-in lasts `auth.session_ttl` (7 days by default) and is renewed once
half of it has passed, but never beyond `auth.session_max_life` (30 days by
default, `0` lifts the cap) from the sign-in. A session is an opaque random
token in an HttpOnly cookie, and only its hash is stored. People see and end
their own sessions on their profile, see [Your account](/guide/account/).

Two-factor authentication (an authenticator app with recovery codes) and
passkeys are each person's own choice, set up on their profile. Turn passkeys
off for the whole install with `auth.passkeys_enabled: false`.
