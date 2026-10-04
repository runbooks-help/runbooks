# Runbooks

A lightweight Go web app that renders operational runbooks as interactive step-by-step pages. Runbooks are plain Markdown files with YAML frontmatter — no database, no CMS.

## Setup

Install [mise](https://mise.jdx.dev/), then let it pull the toolchain:

```bash
mise install
```

This installs Go, templ, gofumpt, and Node.

Optional personal dev values (the git-sync repo, commit identity and API token)
come from a git-ignored `.env` injected by mise. Copy `.env.example` to `.env`
and fill it in; without it the app runs on the committed defaults. SSH remotes use
your SSH agent, so no key file is needed locally.

## Development

```bash
mise run dev
```

Starts a live-reload server on `http://localhost:8091`. templ, CSS, and JS all watch for changes.

## Build

```bash
mise run build
```

Produces a single `runbooks` binary with all assets embedded.

## Install & operate

The app runs from the container image and reads your runbooks from a mounted
directory — see the [install quickstart](https://docs.runbooks.help/install) for
the quickstart. Every setting is an environment variable: the
[configuration reference](https://docs.runbooks.help/configuration) is the full
reference, and [deployment](https://docs.runbooks.help/deployment)
covers storage, backups and upgrading.

Serving a public, crawler-friendly site: set `PUBLIC_URL` to the absolute base
(no trailing slash) to enable canonical/OpenGraph tags and a generated
`/sitemap.xml`, and `SITE_DESCRIPTION` as the fallback meta description.

## Identity

Identity — passkey sign-in, or delegation to an upstream proxy/SSO gateway — is
optional and off by default. To gate reads and attribute writes to named users, see
[identity](https://docs.runbooks.help/identity).

## Writing a runbook

A runbook is a Markdown file with YAML frontmatter under
`content/<system>/<category>/`. The path decides the sidebar hierarchy and the
frontmatter `slug` decides the URL:

```text
content/mysql/replication/mts-deadlock-recovery.md
  → MySQL ▸ Replication, served at /mts-deadlock-recovery
```

```yaml
---
title: Replication Lag (MTS Deadlock)
slug: mts-deadlock-recovery
order: 1
description: One line under the title and on index cards.
symptoms: [Last_SQL_Error]
vars:
  - id: some-ip
    label: Server IP
    var: SERVER_IP
---
```

`## Heading` starts a numbered, tickable step. `layout: sections` in the
frontmatter (or a `---sections` / `---steps` line) renders headings as
unnumbered sections instead, and a single page can mix the two. `vars` become
`{{TOKEN}}` substitutions inside code blocks, filled in the browser and never
sent to the server. `---rollback` starts a rollback section, and
`> [!info]` / `> [!warn]` / `> [!danger]` render notices.

The full format — every frontmatter field, code labels, branch blocks, the
glossary, and the authoring conventions — is
[Writing a runbook](https://docs.runbooks.help/writing-runbooks).

### Index page

`/` is the welcome page: a live filter box, a "Common issues" list, then every
runbook as a card grouped by system/category. The filter matches title,
description, `symptoms`, and the system/category names (phrase first, then
"all words present"). "Common issues" is built
from runbooks marked `common: true` — each contributes its first symptom as the
label and links to the runbook, in sidebar order. Keep `symptoms:` on every
runbook for search, and reserve `common: true` for the shortlist. There is no
separate index file to maintain.

## Agent access

Runbooks are plain Markdown, so a local agent is best served by cloning the
repo and reading `content/` directly — no API, no credentials. For a remote
agent, an identity-enabled instance exposes a read-only surface (`/llms.txt`,
`/<slug>.md`, `/api/runbooks/v1/search`) authorised by a read-scoped API key
created at `/account`. See
[Agent access](https://docs.runbooks.help/agent-access).

## Licence

Runbooks is source-available under [FSL-1.1-MIT](LICENSE): free to self-host,
no competing use, converts to MIT two years after each release. Dependency
licences and attribution live in [DEPENDENCIES.md](DEPENDENCIES.md) and
[NOTICE](NOTICE).
