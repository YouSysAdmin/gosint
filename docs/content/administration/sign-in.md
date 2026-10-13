---
title: Single sign-on
description: "Identity providers over OpenID Connect: adding one, who it admits, linking existing accounts, and 2FA after it."
weight: 40
---

gosint signs people in through any OpenID Connect provider (Keycloak,
Authentik, Entra ID, Okta, Google and others) beside its own passwords.
Providers are not in the configuration file: an administrator adds them in
the console, and their client secrets are sealed in the database.

## Before you start

Set `server.public_url` to the address people reach the console at. The
redirect URI the provider calls back is built from it:

```text
<public_url>/api/auth/oauth/<slug>/callback
```

The Identity providers page shows the exact URI of each provider, with a copy
button, so you can register it at the provider.

## Adding a provider

**Admin > Identity providers > Add a provider**:

| Field | Meaning |
| --- | --- |
| **Name** | What the sign-in page shows: "Continue with *Name*" |
| **Type** | **OpenID Connect**, or **Google**, whose issuer is filled in for you |
| **Slug** | The provider's part of the sign-in and redirect URLs, made from the name when empty |
| **Issuer** | The provider's URL. gosint reads its discovery document from there |
| **Client id**, **Client secret** | The client registered at the provider. The secret is never shown again, leave it empty to keep the stored one |
| **Enabled** | Off, the provider is not offered and its sign-ins are refused |
| **Create an account on first sign-in** | On by default. Off, the provider admits only existing accounts and invited addresses |
| **Require a verified email before linking to an existing account** | On by default |
| **Link to an existing account by a verified email** | Off by default, see below |
| **Allowed domains** | Comma separated email domains, empty admits every domain |

**More settings** holds the rest:

| Field | Meaning |
| --- | --- |
| **Hidden from the sign-in page, reachable by its URL** | For a provider only some people should use |
| **The provider sits on a private address, inside the network** | Allows discovery and token calls to loopback, private and link-local addresses, refused otherwise |
| **Allowed emails** | Comma separated addresses. When set, only these addresses get in, and the domains and groups are not checked |
| **Groups claim**, **Allowed groups** | The claim holding the person's groups, and the groups admitted |
| **Scopes** | Comma separated, `openid email profile` by default |
| **Authorization URL**, **Token URL**, **UserInfo URL** | Manual endpoints, for a provider without discovery |

After saving, **Test** runs discovery and shows the authorization URL it
found, or the error, and warnings about the configuration.

## Who gets in

A sign-in through a provider goes through these steps, in order:

1. **The allow lists.** A verified email when required. Then, when allowed
   emails are set, the address must be one of them and nothing else is
   checked. Otherwise the allowed domains, then the allowed groups.
2. **An account already linked** to this provider and subject signs in.
3. **An existing account with the same email** is linked only when the
   provider has **Link to an existing account by a verified email** on, the
   provider says the address is verified, and the account is **not an
   administrator**. Otherwise the person is told to link the provider from
   their profile, and no second account is ever made for the address.
4. **A new account** is made when **Create an account on first sign-in** is
   on, or a pending [invitation](/administration/accounts/#invitations-by-link)
   names the address. Accounts made through a provider are members, a
   provider never grants administration.

A refusal sends the person back to the sign-in page with a short message
("Your account is not allowed to sign in", "No account exists for that
identity..."). The exact reason is in the server log.

{{< callout type="warning" title="Linking by email trusts the provider" >}}
With linking by email on, whoever controls an address at the provider takes
over the gosint account with that address. Leave it off unless the provider
is the one that issues those addresses, and keep **Require a verified
email** on.
{{< /callout >}}

## Linking from the profile

A person who already has an account links a provider while signed in:
**Profile > Sign-in providers > Link *Name***. The provider's identity is
attached only if it belongs to no other account and the same person is still
signed in when the provider answers. **Unlink** removes it, unless it is the
last way into the account (no password, no passkey, no other provider).

## Two-factor authentication after SSO

An account with an authenticator app turned on is asked for its code after
the provider answers, on the gosint sign-in page, the same as after a
password.

## Removing a provider

Deleting a provider deletes the identities linked through it. An account that
signed in only through that provider has no way in left: give it a reset
link from the Accounts page so it can set a password.

## Outbound requests

Discovery, key and token requests go out from the server. They refuse private
and loopback addresses unless the provider's own **The provider sits on a
private address** box is on, which is how a Keycloak inside the same network
is reached. With the box on, that provider's requests are not checked at all,
so turn it on only for an issuer you run.
