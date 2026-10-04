---
title: Configuration
slug: configuration
order: 1
layout: sections
description: The full configuration reference; every setting is an environment variable.
---

Everything is an environment variable; there is no config file. The defaults are what the container runs with.

## Server

| Var | Default | Meaning |
| --- | --- | --- |
| `PORT` | `8090` | HTTP listen port. |
| `STYLEGUIDE_ENABLED` | `false` | Serve the design-system reference at `/styleguide` and `/styleguide/llms`. |
| `PUBLIC_URL` | _(unset)_ | Absolute site base, e.g. `https://runbooks.help` (trailing slash stripped). Enables canonical/OpenGraph URLs and `/sitemap.xml`. |
| `SITE_DESCRIPTION` | _(unset)_ | Default `<meta name="description">` for pages without their own. |

`/healthz` is always served, unauthenticated, and returns `200 ok`.

## Content

| Var | Default | Meaning |
| --- | --- | --- |
| `CONTENT_DIR` | `content` | Directory the runbooks are read from when `CONTENT_SOURCE=local`. The image ships an empty one; mount your own. |
| `CONTENT_SOURCE` | `local` | `local` (read `CONTENT_DIR`) or `git` (clone the remote into a cache and read it). |
| `CONTENT_GIT_PATH` | `.` | Directory within the repo to read when `CONTENT_SOURCE=git`. |
| `CONTENT_GIT_CACHE` | `data/content` | Where the git clone lives; reused across restarts. |
| `CONTENT_REFRESH_TOKEN` | _(unset)_ | Bearer token for `POST /api/content/v1/refresh` when identity is off; unset disables the endpoint (it is admin-only when identity is on). |
| `CONTENT_REFRESH_INTERVAL` | _(unset)_ | Opt-in background refresh, e.g. `5m`. Go duration; unset or `0` is off. Values below `1m` are rejected at startup. Applies to any source (`local` re-read, `git` fetch first). |

With `CONTENT_SOURCE=git`, the remote and credential come from the `GITSYNC_*` variables below: content at the repo root, notes records in `GITSYNC_BASE_PATH`, which the content walk skips.

## Identity

Identity is **off** until `IDENTITY_DB_DRIVER` is set; with no driver the app is public as before.

| Var | Default | Meaning |
| --- | --- | --- |
| `IDENTITY_DB_DRIVER` | _(unset)_ | `sqlite`, `mysql` or `postgres`. Unset = identity off. |
| `IDENTITY_DB_DSN` | `file:./data/runbooks.db` (sqlite) | Driver DSN or SQLite file path. |
| `IDENTITY_PUBLIC_URL` | _(required when on)_ | Scheme + host; derives the WebAuthn RP ID and origin. Effectively immutable once passkeys exist. |
| `IDENTITY_BOOTSTRAP_TOKEN` | _(generated and logged once)_ | Guards `/setup` for the first admin. |
| `IDENTITY_RECOVERY_TOKEN` | _(unset)_ | Break-glass re-enrolment for the sole admin; `/recovery` is disabled when unset. |
| `IDENTITY_TRUST_PROXY_AUTH` | `false` | Trust an identity asserted by an upstream proxy/SSO gateway. Only safe when the app is unreachable except through that proxy. |
| `IDENTITY_PROXY_USER_HEADER` | `Auth-Request-Email` | Header carrying the login/email. |
| `IDENTITY_PROXY_NAME_HEADER` | _(unset)_ | Header carrying the display name. |
| `IDENTITY_SECURE_COOKIES` | `true` | Cookie `Secure` flag; relax only for local HTTP. |
| `IDENTITY_SESSION_TTL` | `720h` | Absolute session lifetime. |
| `IDENTITY_SESSION_IDLE` | `168h` | Idle session lifetime. |

`IDENTITY_BOOTSTRAP_TOKEN` is ignored once an admin exists. There is no API to rotate the bootstrap or recovery tokens: change the value and restart.

## Git sync

Sync is enabled only when a repo **and** a credential **and** an endpoint auth are configured. The endpoint auth is a user session when identity is on (an unauthenticated request is refused), or `GITSYNC_API_TOKEN` when identity is off.

A credential is an HTTPS token (`GITSYNC_TOKEN`), an explicit SSH private key (`GITSYNC_SSH_KEY`), or, for an `ssh://` or scp-style remote, the ambient SSH agent. With no key and no token, `go-git` authenticates SSH remotes through `$SSH_AUTH_SOCK` (ssh-agent, 1Password, …), so the common local setup needs no credential variable at all. Set `GITSYNC_SSH_KEY` only where there is no agent: a deployment or CI job that mounts a key and points the variable at its path. See [docs/operations/deployment.md](../operations/deployment.md#ssh-credentials) for the container recipe.

| Var | Default | Meaning |
| --- | --- | --- |
| `GITSYNC_REPO` | _(unset)_ | Remote URL: any host, HTTPS or SSH. Unset disables sync. |
| `GITSYNC_BRANCH` | `main` | Target branch. |
| `GITSYNC_BASE_PATH` | `runbook_runs` | Directory prefix for records (skipped by the content walk when it sits inside the content tree). |
| `GITSYNC_AUTHOR_NAME` | _(required)_ | Fallback commit identity, when the user has no email. |
| `GITSYNC_AUTHOR_EMAIL` | _(required)_ | Fallback commit email. |
| `GITSYNC_USERNAME` | `oauth2` | HTTPS basic-auth username (token as password). |
| `GITSYNC_TOKEN` | _(unset)_ | HTTPS access token. |
| `GITSYNC_SSH_KEY` | _(unset)_ | Private key path for SSH remotes; omit to use the ambient SSH agent. |
| `GITSYNC_API_TOKEN` | _(unset)_ | Shared bearer that gates the sync endpoint, for CI/automation and for identity-off deployments. |

Each sync writes a new, immutable snapshot directory under the base path, `<GITSYNC_BASE_PATH>/<YYYY-MM-DD>T<HHMMSSZ>-<slug>/` (UTC), so every sync is preserved; a re-sync with no changes is skipped.

With identity on, a signed-in user's commit is authored as that user (`DisplayName <Email>`); `GITSYNC_AUTHOR_*` applies only when the user has no email, and `GITSYNC_API_TOKEN` is the non-human fallback.

The git transport is `go-git` (pure Go); no system `git` binary is needed. SSH remotes are verified against `known_hosts` (`SSH_KNOWN_HOSTS`, then `~/.ssh/known_hosts` / `/etc/ssh/ssh_known_hosts`); there is no `accept-new`, so the host key must already be present.
