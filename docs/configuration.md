# Configuration

Everything is an environment variable — there is no config file. The defaults are
what the container runs with.

## Server

| Var | Default | Meaning |
|---|---|---|
| `PORT` | `8090` | HTTP listen port. |
| `STYLEGUIDE_ENABLED` | `false` | Serve the design-system reference at `/styleguide` and `/styleguide/llms`. |

`/healthz` is always served, unauthenticated, and returns `200 ok`.

## Content

| Var | Default | Meaning |
|---|---|---|
| `CONTENT_DIR` | `content` | Directory the runbooks are read from when `CONTENT_SOURCE=local`. The image ships an empty one — mount your own. |
| `CONTENT_SOURCE` | `local` | `local` (read `CONTENT_DIR`) or `git` (clone the remote into a cache and read it). |
| `CONTENT_GIT_PATH` | `.` | Directory within the repo to read when `CONTENT_SOURCE=git`. |
| `CONTENT_GIT_CACHE` | `data/content` | Where the git clone lives; reused across restarts. |

With `CONTENT_SOURCE=git`, the remote and credential come from the `GITSYNC_*`
variables below — content at the repo root, notes records in `GITSYNC_BASE_PATH`,
which the content walk skips.

## Identity

Identity is **off** until `IDENTITY_DB_DRIVER` is set; with no driver the app is
public as before.

| Var | Default | Meaning |
|---|---|---|
| `IDENTITY_DB_DRIVER` | *(unset)* | `sqlite`, `mysql` or `postgres`. Unset = identity off. |
| `IDENTITY_DB_DSN` | `file:./data/runbooks.db` (sqlite) | Driver DSN or SQLite file path. |
| `IDENTITY_PUBLIC_URL` | *(required when on)* | Scheme + host; derives the WebAuthn RP ID and origin. Effectively immutable once passkeys exist. |
| `IDENTITY_BOOTSTRAP_TOKEN` | *(generated and logged once)* | Guards `/setup` for the first admin. |
| `IDENTITY_RECOVERY_TOKEN` | *(unset)* | Break-glass re-enrolment for the sole admin; `/recovery` is disabled when unset. |
| `IDENTITY_TRUST_PROXY_AUTH` | `false` | Trust an identity asserted by an upstream proxy/SSO gateway. Only safe when the app is unreachable except through that proxy. |
| `IDENTITY_PROXY_USER_HEADER` | `Auth-Request-Email` | Header carrying the login/email. |
| `IDENTITY_PROXY_NAME_HEADER` | *(unset)* | Header carrying the display name. |
| `IDENTITY_SECURE_COOKIES` | `true` | Cookie `Secure` flag; relax only for local HTTP. |
| `IDENTITY_SESSION_TTL` | `720h` | Absolute session lifetime. |
| `IDENTITY_SESSION_IDLE` | `168h` | Idle session lifetime. |

`IDENTITY_BOOTSTRAP_TOKEN` is ignored once an admin exists. There is no API to
rotate the bootstrap or recovery tokens: change the value and restart.

## Git sync

Sync is enabled only when a repo **and** a credential **and** an endpoint auth
are configured. The endpoint auth is a user session when identity is on (an
unauthenticated request is refused), or `GITSYNC_API_TOKEN` when identity is off.

| Var | Default | Meaning |
|---|---|---|
| `GITSYNC_REPO` | *(unset)* | Remote URL — any host, HTTPS or SSH. Unset disables sync. |
| `GITSYNC_BRANCH` | `main` | Target branch. |
| `GITSYNC_BASE_PATH` | `runbook_runs` | Directory prefix for records (skipped by the content walk when it sits inside the content tree). |
| `GITSYNC_AUTHOR_NAME` | *(required)* | Fallback commit identity, when the user has no email. |
| `GITSYNC_AUTHOR_EMAIL` | *(required)* | Fallback commit email. |
| `GITSYNC_USERNAME` | `oauth2` | HTTPS basic-auth username (token as password). |
| `GITSYNC_TOKEN` | *(unset)* | HTTPS access token. |
| `GITSYNC_SSH_KEY` | *(unset)* | Private key path for SSH remotes. |
| `GITSYNC_API_TOKEN` | *(unset)* | Shared bearer that gates the sync endpoint, for CI/automation and for identity-off deployments. |

With identity on, a signed-in user's commit is authored as that user
(`DisplayName <Email>`); `GITSYNC_AUTHOR_*` applies only when the user has no
email, and `GITSYNC_API_TOKEN` is the non-human fallback.
