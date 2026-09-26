# Organizations, roles, SSO and quotas

Every account gets a personal organization; create more from the switcher.
Owners and admins manage members under **Members** and **Settings**.

## Members and invites

Add someone who already has an account by username, or create a single-use
invite link for an email. Roles: owner, admin, member, viewer (see
[Concepts](../start/concepts.md)). An organization always keeps one owner.

## SSO (OpenID Connect)

**Settings → SSO & SCIM**: issuer URL, client ID and secret, the email domain,
and the role new members get. Register `https://<dashboard>/api/auth/sso/callback`
with your identity provider. People sign in with "Sign in with SSO" and their
work email, and join the organization automatically.

## SCIM 2.0

Generate a bearer token (shown once) and point your identity provider at
`https://<dashboard>/scim/v2`. `Users` supports list with `userName eq` filters,
create, get, `PATCH`/`PUT` of `active`, and delete. Deactivated people lose the
membership and, if it was their last organization, their sessions.

## Quotas

The **organization cap** (set by instance admins) bounds CPU, memory, storage,
apps, databases and sandboxes; the **per-member allowance** (set by org admins)
bounds each member and viewer inside it. Zero means unlimited. Creating or
resizing anything checks both.

## Audit log

Sign-ins aside, every change is recorded: who, what, target, when. Filter by
action, person or target under **Settings → Audit log**.

## Passkeys

**Account → Passkeys & password** registers WebAuthn passkeys (Touch ID, Windows
Hello, phones, security keys). Passkeys need HTTPS or localhost; plain-HTTP LAN
installs use passwords.
