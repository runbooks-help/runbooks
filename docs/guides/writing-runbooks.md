---
title: Writing a runbook
slug: writing-runbooks
order: 1
layout: sections
description: The runbook file format, covering frontmatter, steps and sections, code, notices, variables and the glossary.
---

A runbook is a Markdown file with YAML frontmatter, under the content root. The file path and the frontmatter decide where it appears and at what URL.

## Where files live

`content/<system>/<category>/<file>.md`:

```text [Content layout]
content/
  mysql/
    replication/
      mts-deadlock-recovery.md   → MySQL ▸ Replication
  kubernetes/
    networking/
      k3s-log-proxy-502.md
```

- `<system>` is a top-level sidebar group (the directory name, title-cased).
- `<category>` is a subheading under the system.
- A file directly in `content/<system>/` renders under the system with no subheading.

Display names and order live in `content/_meta.yml` at the top of the tree: an ordered list where position is the order:

```yaml
- mysql:
    title: MySQL
    categories: [replication, failover, backup]
- kubernetes
- backend
```

An item is a bare directory name or `name: {title, categories}`; a category may carry a title the same way. Unlisted directories are appended after the listed ones, alphabetically. Within a category, runbooks sort by the optional frontmatter `order:` (lower first; absent sorts last), then by title. Use `order:` to pull the most likely on-call runbook to the top of its category.

## Frontmatter

Only `title` and `slug` are required; everything else is optional.

```yaml
---
title: Replication Lag (MTS Deadlock)
slug: mts-deadlock-recovery
order: 1
description: One-line summary under the title and on index cards.
symptoms:
  - Last_SQL_Error
  - duplicate entry error 1062
common: true
notice: "READ EVERY STEP BEFORE DOING ANYTHING!"
acknowledge: "This drops and rebuilds the replica."
vars:
  - id: some-ip
    label: Server IP
    var: SERVER_IP
    placeholder: "e.g. 10.0.0.5"
    hint: How to find this value (plain-text popover on hover)
  - id: db-password
    label: DB password
    var: DB_PASS
    secret: true
---
```

| Field | What it does |
| --- | --- |
| `title` | Sidebar label and `<h1>`. |
| `slug` | The URL, `/<slug>`, **not** the filename. |
| `order` | Position within its category (lower first). |
| `description` | One-line summary under the `<h1>` and on index cards. |
| `symptoms` | Extra keywords the index filter and search match. |
| `common` | `true` pins it into the "Common issues" shortlist. |
| `notice` | A passive banner above the body. |
| `acknowledge` | Marks the runbook destructive: the page is gated behind an "I understand" dialog until accepted (once per session). |
| `vars` | Runtime inputs; see below. |

Keep `symptoms:` on every runbook: the index matches title, description, symptoms and the system/category names, and the body search reaches through the file.

## Inputs with `vars`

`vars` render a panel of inputs. Each value substitutes into code blocks as a `{{TOKEN}}`, where the token is uppercase A–Z and underscore only:

```bash [Use the inputs]
ssh "{{SERVER_IP}}"
systemctl restart "{{SERVICE}}"
```

- Give the diagnostic lookup to a `hint` rather than a literal `<placeholder>` inside the code: angle brackets in a fence look like real syntax.
- Set `secret: true` to render a password field.
- Substitution happens in the browser, so values **never leave the page**.

## Steps and sections

A `##` heading is a **step** by default: numbered, tickable and collapsible. Add `layout: sections` to the frontmatter to render headings as **sections** instead (anchored, unnumbered, always open), or switch mid-page with `---sections` / `---steps` on their own line. The GitHub-safe comment forms (`<!-- sections -->` / `<!-- steps -->`) do the same and stay invisible in a rendered Markdown view (GitHub, any CommonMark renderer).

The two mix freely: a page can explain, hand over a numbered procedure, then continue explaining. Numbering counts steps only, the Contents list and prev/next do not care which a heading is, and `###` subheadings are anchored too.

<!-- steps -->

## Resolve the lag

The numbered steps from here to the next separator are a procedure, with the same tick boxes and collapse behaviour as every other step:

1. Confirm the lag on the replica.
2. Stop the SQL thread.
3. Clear the deadlock and restart the thread.

<!-- sections -->

## Code blocks

Add a language and an optional bracketed label; the label renders a header with a per-block "done" checkbox. The language tag must be lowercase letters:

```sql [Run on mysql-prod-primary]
SELECT @@read_only, @@global.gtid_executed;
```

## Notices

```markdown
> [!info] Informational note. [!warn] Something to be careful about. [!danger] This causes an outage.
```

The GitHub alert keywords (`NOTE`, `TIP`, `IMPORTANT`, `WARNING`, `CAUTION`) are accepted and mapped onto info/warn/danger, and the marker is case-insensitive.

## Decision points

A branch block presents the choices at a fork in the procedure:

```markdown
> [!branch]
>
> - All good → [Next step](#next-step)
> - Something broke → [Roll back](#rollback)
```

## Rollback

`---rollback` (or `<!-- rollback -->`) on its own line starts a separate rollback section: danger-styled, unnumbered, and read as "what to do if it broke". Everything after it is rollback steps.

## Glossary

One file at the content root, `content/_glossary.yml`, defines the shared terms for the instance. A configured term highlights wherever prose renders (step and section titles, notices, lists, tables) and shows its expansion, detail and optional link on hover or focus. Code is never touched.

```yaml
- term: MTS
  expansion: Multi-Threaded Replica
  description: MySQL replica applying transactions in parallel workers.
  link: https://internal/glossary#mts
```

`term` and `expansion` are required; a missing one fails startup. Matching is exact, case-sensitive and whole-word, so `IT` never matches "it".

## Conventions

- **Titles lead with the symptom, not the mechanism**: "Replication Lag (MTS Deadlock)", not "MTS Deadlock Recovery". This is a runbook read under duress.
- **One concern per step.** If a step needs a sub-decision, use `###` or a branch block rather than burying it in prose.
- **Destructive procedures declare `acknowledge:`** so the reader must confirm before the page is usable; `notice:` stays a passive banner.

## Where next

- [Configuration reference](../reference/configuration.md)
- [Deployment](../operations/deployment.md)
- [Agent access](../operations/agent-access.md)
