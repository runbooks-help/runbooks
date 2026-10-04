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

## Configuration & operations

Every setting is an environment variable — see
[docs/configuration.md](docs/configuration.md) for the full reference, and
[docs/operations.md](docs/operations.md) for storage, backups and upgrading.

Serving a public, crawler-friendly site: set `PUBLIC_URL` to the absolute base
(no trailing slash) to enable canonical/OpenGraph tags and a generated
`/sitemap.xml`, and `SITE_DESCRIPTION` as the fallback meta description.

## Identity

Identity — passkey sign-in, or delegation to an upstream proxy/SSO gateway — is
optional and off by default. To gate reads and attribute writes to named users, see
[docs/identity.md](docs/identity.md).

## Writing a runbook

Create a `.md` file under `content/<system>/<category>/`. The path decides the sidebar hierarchy:

```
content/
  mysql/
    replication/
      mts-deadlock-recovery.md   → MySQL ▸ Replication
  kubernetes/
    networking/
      k3s-log-proxy-502.md
```

`<system>` is the top-level sidebar heading (the directory name, title-cased — `mysql` is
special-cased to `MySQL` in `parser.systemLabels`). `<category>` is the subheading.
A file placed directly in `content/<system>/` renders under the system with no subheading.
The filename doesn't matter — the slug comes from frontmatter.

Sidebar order is explicit, not alphabetical: `parser.systemOrder` and
`parser.categoryOrder` list systems and (per system) categories in operational
priority order; anything unlisted follows alphabetically. Within a category,
runbooks sort by the optional frontmatter `order:` (lower first, default 100),
then by title — use it to pull a symptom-first runbook to the top.

### Frontmatter

```yaml
---
title: My Runbook
slug: my-runbook
order: 1           # optional: sort position within the category (lower first)
description: One-line summary shown under the title and on the index card.
symptoms:          # optional: extra keywords the index filter matches
  - Last_SQL_Error
  - duplicate entry error 1062
common: true       # optional: pin into the index "Common issues" shortlist
notice: "Production traffic is affected by this procedure. Read all steps before starting."
acknowledge: "This drops and rebuilds the replica. Take it out of the load balancer first."  # gates the page behind an I-understand dialog
vars:
  - id: some-ip
    label: Server IP
    var: SERVER_IP
    hint: How to find this value (shown on hover)
  - id: db-password
    label: DB password
    var: DB_PASS
    secret: true   # renders as a password input
---
```

`vars` defines runtime inputs. Values are substituted into code blocks with `{{VAR_NAME}}` — the browser fills these in; they never leave the page.

### Index page

`/` is the welcome page: a live filter box, a "Common issues" list, then every
runbook as a card grouped by system/category. The filter matches title,
description, `symptoms`, and the system/category names (phrase first, then
"all words present"). "Common issues" is built
from runbooks marked `common: true` — each contributes its first symptom as the
label and links to the runbook, in sidebar order. Keep `symptoms:` on every
runbook for search, and reserve `common: true` for the shortlist. There is no
separate index file to maintain.

### Steps

`## Heading` starts a new step:

```markdown
## Connect to the server

```bash
ssh root@{{SERVER_IP}}
` ``
```

### Sections

A `##` heading is a **step** by default: numbered, tickable and collapsible.
Add `layout: sections` to the frontmatter to render headings as **sections**
instead — anchored, unnumbered and always open — or switch mid-page with
`---sections` / `---steps` on their own line. The GitHub-safe comment forms
(`<!-- sections -->` / `<!-- steps -->`) do the same and stay invisible in a
rendered Markdown view.

The two mix freely: a page can explain, hand you a numbered procedure, then keep
explaining. Numbering counts steps only, the page lead renders either way, and
the Contents list shows both.

### Code blocks

Optionally add a language and a label after the opening fence:

```
` ``sql [Run on mysql-prod-primary]
SELECT @@read_only;
` ``
```

### Notices

```markdown
> [!info] Informational note.
> [!warn] Something to be careful about.
> [!danger] This causes an outage.
```

### Branch blocks (decision points)

```markdown
> [!branch]
> - All good → [Next step](#next-step)
> - Something broke → [Roll back](#rollback)
```

### Rollback section

Add `---rollback` on its own line to start a separate rollback steps section. Everything after it is treated as rollback steps, not main steps.

```markdown
---rollback

## Undo the freeze

` ``sql
SET GLOBAL read_only = 0;
` ``
```

### Glossary

Terms live in one file at the content root, `content/_glossary.yml`. A configured
term highlights wherever prose renders — step and section titles, notices, lists,
tables — and shows its expansion, detail and optional link on hover or focus.
Code is never touched.

```yaml
- term: MTS
  expansion: Multi-Threaded Replica
  description: MySQL replica applying transactions in parallel workers.
  link: https://internal/glossary#mts   # optional
```

`term` and `expansion` are required; a missing one fails startup.

## Agent access

Runbooks are plain Markdown, so a local agent is best served by cloning the
repo and reading `content/` directly — no API, no credentials. For a remote
agent, an identity-enabled instance exposes a read-only surface (`/llms.txt`,
`/<slug>.md`, `/api/runbooks/v1/search`) authorised by a read-scoped API key
created at `/account`. See [docs/agent-access.md](docs/agent-access.md).

## Troubleshooting

See [docs/troubleshooting.md](docs/troubleshooting.md) — notably, dark colours looking
washed out on an HDR display in Chromium-based browsers, and the workarounds.

## Licence

Runbooks is source-available under [FSL-1.1-MIT](LICENSE): free to self-host,
no competing use, converts to MIT two years after each release. Dependency
licences and attribution live in [DEPENDENCIES.md](DEPENDENCIES.md) and
[NOTICE](NOTICE).
