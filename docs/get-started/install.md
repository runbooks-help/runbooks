---
title: Install & quickstart
slug: install
order: 2
layout: sections
description: Run the container, mount your runbooks, and see your first page.
---

## Run the container

The app ships as a container image and carries no content. You mount a directory of Markdown runbooks.

```bash [Start Runbooks]
docker run -d --name runbooks \
  -p 8090:8090 \
  -v "$PWD/content:/app/content" \
  -v runbooks-data:/app/data \
  ghcr.io/runbooks-help/runbooks:latest
```

Open [localhost:8090](http://localhost:8090).

- `/app/content` is the content root (`CONTENT_DIR`). If it is empty the index renders a welcome page; add a `.md` file to populate it.
- `/app/data` holds the SQLite identity database when identity is on. It is unused otherwise, but mount it so a later upgrade keeps its data.
- The container runs as a non-root user and exposes `/healthz`.

Pin a version tag (`:vX.Y.Z`) rather than `:latest` for a production deployment.

## Your first runbook

Create `content/mysql/backup/verify-backup.md`:

```markdown
---
title: Verify the nightly backup
slug: verify-backup
description: Check that last night's dump exists and restores.
---

## Check the dump

The dump should be from tonight and non-trivial in size.
```

The path puts it under **MySQL › Backup** in the sidebar, and the URL is `/verify-backup` (from the slug, not the filename). Content is read from disk at startup: no rebuild, no restart. Add a code block, a notice or `vars` as you need them: see [Writing a runbook](../guides/writing-runbooks.md).

## Configuration

Every setting is an environment variable; the full list is [Configuration](../reference/configuration.md). The common ones:

| Var | Default | Meaning |
| --- | --- | --- |
| `PORT` | `8090` | HTTP listen port. |
| `CONTENT_DIR` | `content` | Directory the runbooks are read from. |
| `PUBLIC_URL` | _(unset)_ | Absolute site base; enables canonical/OpenGraph tags and `/sitemap.xml`. |
| `IDENTITY_DB_DRIVER` | _(unset)_ | `sqlite`, `mysql` or `postgres`; unset leaves the app public. |

## Build from source

The image is the supported distribution. The FSL permits building from source:

```bash [Build and run]
mise install
mise run build
./runbooks            # serves on :8090, reads ./content
```

## Where next

- [Writing a runbook](../guides/writing-runbooks.md)
- [Deployment](../operations/deployment.md)
- [Identity](../operations/identity.md)
