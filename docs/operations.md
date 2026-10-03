# Operations

## Storage

- **Runbooks** are plain markdown read from a content **source** at runtime — not
  stored in a database, and not part of a backup of it. By default that is a local
  directory (`CONTENT_DIR`, default `content`); set `CONTENT_SOURCE=git` to read
  from a repository instead, cloned into `CONTENT_GIT_CACHE` (records under
  `GITSYNC_BASE_PATH` are skipped). The image ships an empty directory (it never
  bundles content); mount yours there, or point the source wherever you like.
- **Identity** (users, passkeys, sessions, invites, audit events) lives in the
  configured database. SQLite is the default and keeps everything in one file at
  `file:./data/runbooks.db`.
- In the container the working directory is `/app`, so the SQLite file is
  `/app/data/runbooks.db`. Mount a volume at `/app/data` to persist it.
- The schema is applied at startup from embedded, idempotent migrations. There is
  no external migration tool and no migration step: **to upgrade, replace the
  binary or image and restart.**
- `IDENTITY_PUBLIC_URL` is effectively immutable once passkeys exist — changing
  the host invalidates every credential.

## Content

Runbooks are read from a **source**, selected by `CONTENT_SOURCE`:

- `local` (default) — read `CONTENT_DIR` directly. Mount your content there, or
  point it wherever you like.
- `git` — clone `GITSYNC_REPO` (branch `GITSYNC_BRANCH`, credential from the
  `GITSYNC_*` vars) into `CONTENT_GIT_CACHE` and read `CONTENT_GIT_PATH` inside
  it. Content sits at the repo root; the notes records path (`GITSYNC_BASE_PATH`)
  is skipped by the content walk.

### Startup order

- **Cache present:** the cached checkout is parsed and served immediately, then
  the remote is fetched in the background. An unreachable remote does not delay
  or fail boot — the last successful fetch keeps serving.
- **No cache:** the clone happens before the server listens. Boot fails only
  when there is neither a cache nor a successful fetch.

A failed fetch or a parse error keeps the current in-memory snapshot
(last-good); it never serves a half-loaded set.

### Refreshing content

`POST /api/content/v1/refresh` fetches the source, parses it and swaps the
snapshot atomically, so no request observes a partially-loaded set and in-flight
requests keep the set they started on.

- With identity on it is **admin-only** — a signed-in admin; an API key is
  read-only by construction and cannot call it.
- With identity off it requires `Authorization: Bearer $CONTENT_REFRESH_TOKEN`,
  and is disabled (`403`) when no token is configured.

A local source is re-read in place; only a git source fetches. Refresh is
manual — there is no poll interval or webhook.

### Cache

`CONTENT_GIT_CACHE` (default `data/content`) is a single inspectable directory
under the data volume, and is an ordinary git clone. It is not a backed-up
artifact: a restart with the remote reachable re-fetches, and an empty cache
just means a fresh clone at boot.

## Backing up SQLite

Online, with the app running, take a consistent snapshot with `VACUUM INTO`:

```bash
sqlite3 ./data/runbooks.db "VACUUM INTO '/backup/runbooks-$(date +%F).db'"
```

The result is a single, self-contained database file. Restore by pointing
`IDENTITY_DB_DSN` at the copy and restarting. With the app stopped, a plain file
copy of `data/runbooks.db` is equally valid.

Verified once against this procedure: create a database, `VACUUM INTO` a copy,
open the copy and read a row back — the snapshot restores cleanly.

## MySQL and PostgreSQL

Use the database's own tooling — `mysqldump` / `pg_dump`, or the managed
service's snapshots. Runbooks ships no backup code. The identity tables are:

```
users, credentials, sessions, invites, webauthn_challenges, auth_events
```

There are no foreign keys, so the set is self-contained; restoring the database
restores identity. Runbook content is unaffected by a database restore.

## Health and readiness

`GET /healthz` returns `200 ok`, unauthenticated, on an identity-on instance too —
wire container or load-balancer probes to it.

## Version

`runbooks --version` prints the build stamp — `runbooks vX.Y.Z` for a tagged
build, the short commit otherwise. Tagged images also carry
`org.opencontainers.image.version` and `…revision` labels:

```bash
docker inspect --format '{{index .Config.Labels "org.opencontainers.image.version"}}' \
  ghcr.io/ladydascalie/runbooks:<tag>
```
