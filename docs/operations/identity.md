---
title: Identity
slug: identity
order: 2
layout: sections
description: Passkey sign-in, proxy delegation, and gating reads on named users.
---

Runbooks can require named users, so that reads are gated and writes are attributed to a person. It is **off by default**: with no identity database configured the app is public and the sync API is gated by `GITSYNC_API_TOKEN`.

Sign-in is by **passkey** (WebAuthn): no password, no shared secret, no email. Registration is invite-only, and the first admin is created with a bootstrap token.

## Enabling

| Var | Meaning | Default |
| --- | --- | --- |
| `IDENTITY_DB_DRIVER` | `sqlite`, `mysql` or `postgres` | _(unset, identity off)_ |
| `IDENTITY_DB_DSN` | DSN / SQLite file path | `file:./data/runbooks.db` (sqlite only) |
| `IDENTITY_PUBLIC_URL` | Scheme + host; derives the WebAuthn RP ID and origin | required when on |
| `IDENTITY_BOOTSTRAP_TOKEN` | Guards `/setup` for the first admin | generated and logged once |
| `IDENTITY_RECOVERY_TOKEN` | Break-glass re-enrolment for the last admin | _(unset, `/recovery` disabled)_ |
| `IDENTITY_SESSION_TTL` / `IDENTITY_SESSION_IDLE` | Absolute / idle session lifetime | `720h` / `168h` |
| `IDENTITY_SECURE_COOKIES` | Cookie `Secure` flag | `true` |

`IDENTITY_PUBLIC_URL` is effectively immutable: changing the host after passkeys exist invalidates every credential.

Open `/setup` and present the bootstrap token to enrol the first admin; from there, `/admin` issues single-use invite links (`/invite/<token>`). With identity on, every page (including each runbook) redirects to `/login` when unauthenticated.

## Delegating to an upstream proxy

Instead of passkeys, the app can trust an identity asserted by a reverse proxy or SSO gateway in front of it:

| Var                          | Meaning                            | Default              |
| ---------------------------- | ---------------------------------- | -------------------- |
| `IDENTITY_TRUST_PROXY_AUTH`  | Trust the upstream identity header | `false`              |
| `IDENTITY_PROXY_USER_HEADER` | Header carrying the login/email    | `Auth-Request-Email` |
| `IDENTITY_PROXY_NAME_HEADER` | Header carrying the display name   | _(unset)_            |

On first sight the asserted identity is provisioned as a `member`; admin is never granted from a header. A request is resolved from the session cookie first, then the header, so a proxied request is authenticated without a login.

> [!WARNING] Only enable this when the instance is **unreachable except through the proxy**, which must set and strip the header. A directly reachable instance is an impersonation hole: anyone who can reach it can set the header themselves.

### Tailscale Serve

`tailscale serve` injects `Tailscale-User-Login` and `Tailscale-User-Name` on requests it proxies, so a Tailnet-fronted instance maps directly onto proxy delegation:

```bash
IDENTITY_DB_DRIVER=sqlite \
IDENTITY_DB_DSN=file:./data/runbooks.db \
IDENTITY_PUBLIC_URL=https://runbooks.example.ts.net \
IDENTITY_TRUST_PROXY_AUTH=true \
IDENTITY_PROXY_USER_HEADER=Tailscale-User-Login \
IDENTITY_PROXY_NAME_HEADER=Tailscale-User-Name \
  ./runbooks
```

with the app listening on loopback and `tailscale serve` in front of it. Caveats:

- Serve omits the identity headers for **tagged devices** and for **Funnel** traffic, so those requests are anonymous.
- Non-ASCII values may be RFC 2047 "Q"-encoded (`=?utf-8?q?...?=`).
- Behind a Kubernetes L7 operator `Ingress`, the proxy forwards to a cluster-reachable Service rather than localhost, so the equivalent of "loopback only" is a NetworkPolicy admitting only the ingress proxy pods to the app port.

## Attribution

With identity on, `/api/git-sync/v1` requires an authenticated session (or proxy assertion), and the commit author is the signed-in user: name and email from the user record. `GITSYNC_API_TOKEN` remains the non-human automation fallback for CI and scripts. Runbook acknowledgements are recorded against the authenticated user.
