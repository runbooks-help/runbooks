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
