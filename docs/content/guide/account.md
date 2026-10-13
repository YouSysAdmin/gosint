---
title: Your account
description: "Your profile, password, two-factor authentication, recovery codes, passkeys, sessions and linked sign-in providers."
weight: 100
---

Open **Profile** from the account menu in the top right corner (your
initials or picture).

## Profile

Set your **First name**, **Last name** and an **Avatar URL**, a picture
shown beside your name. Your name is what teammates see in "added by" and
"just edited". Leave the avatar empty to remove it.

## Security

The **Security** section holds everything that protects your sign-in. An
account that signs in through an identity provider (single sign-on) has no
password or second factor here, those are managed by the provider.

### Password

**Change password** asks for your current password and a new one of at
least 8 characters. Changing it signs out every other session, the one you
are using stays.

Five wrong passwords in a row lock the account for 15 minutes.

### Two-factor authentication

With two-factor on, signing in asks for a six digit code from an
authenticator app on your phone after your password.

1. Confirm your password to begin.
2. Scan the QR code with your authenticator app, or type the secret in by
   hand.
3. Enter a code from the app. Two-factor is now on, and every other session
   is signed out.
4. Save the **recovery codes** gosint shows you.

Each code works once. Five wrong codes in a row lock the second step for
15 minutes. To turn two-factor off, enter a current code and press **Turn
off**.

### Recovery codes

Recovery codes sign you in when your authenticator is gone. You get ten,
and each works once in place of a code, in the same **Authenticator or
recovery code** field at sign-in.

They are shown **once**, when two-factor is turned on or when you press
**Generate new codes** (behind your password). gosint keeps only a hash of
them. Press **Copy the codes**, store them somewhere that is not your
phone, then **I saved them**. The panel says how many are left.

### Passkeys

A passkey signs you in with your fingerprint, face or device PIN, with no
password and no code, and cannot be phished. **Add a passkey**, give it a
name so you can tell it apart later (like "MacBook Touch ID"), confirm with
your password, and follow your browser.

On the sign-in page, **Sign in with a passkey** then appears. **Remove**
takes a passkey off, again behind your password.

Passkeys are for local accounts, when the administrator has them enabled,
in a browser that supports them. The desktop app does not offer them.

### Sign-in providers

When the administrator set up single sign-on, **Sign-in providers** lists
the providers linked to your account. **Link** one to sign in through it as
well, **Unlink** to remove it. gosint refuses to unlink the last way you
have to sign in.

### Signed in

**Signed in** lists your sessions: the browser, the address and when each
started. The one you are using is marked. **Revoke** ends another
session, **Sign out everywhere else** ends all of them but this one.

A session lasts a week from its last use and is renewed while you work,
up to 30 days from the sign-in (an administrator can change both).

## Forgot your password

**Forgot your password?** on the sign-in page mails a reset link to your
address. The link works once, within 30 minutes. Setting a new password
through it signs out every session.

When the server sends no mail, ask an administrator: they can hand you a
reset link from the admin panel.
