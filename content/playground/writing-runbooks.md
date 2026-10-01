---
title: How to write a runbook (starter guide)
slug: writing-runbooks
order: 0
description: A working example of the runbook format and the authoring conventions — safe to delete once your own runbooks exist.
symptoms:
  - authoring
  - frontmatter
  - runbook syntax
  - how to write a runbook
notice: "Starter guide — this page teaches the format and is safe to delete."
vars:
  - id: guide-host
    label: Example target host
    var: HOST
    placeholder: "e.g. db-2.prod.internal"
    hint: |-
      Feeds the {{HOST}} token in the examples below.
      Values are substituted in the browser and never sent to the server.
  - id: guide-service
    label: Example service name
    var: SERVICE
    placeholder: "e.g. checkout-api"
    hint: A second input, to show two `vars` side by side.
---

This page is a runbook about writing runbooks. Read it, copy the patterns, then delete it.

## Where runbooks live

The filesystem is the taxonomy — there is no database and no CMS. A runbook is a Markdown file with YAML frontmatter:

```text [Content layout]
content/<system>/<category>/<file>.md
```

- `<system>` becomes a top-level sidebar group (the directory name, title-cased).
- `<category>` becomes a subheading under the system.
- A file directly under the system (`content/<system>/<file>.md`) has no subheading.
- The URL is `/<slug>`, and `slug` comes from frontmatter — not the filename.

Directory names map to display names and sidebar order in `parser/parser.go` (`systemLabels`, `systemOrder`, `categoryOrder`). Within a category, runbooks sort by the optional `order:` field, then by title. Content is read from disk at runtime, so edits need no rebuild — just refresh.

## Frontmatter

Only `title` and `slug` are required; everything else is optional.

| Field | What it does |
| --- | --- |
| `title` | Sidebar label and `<h1>`. |
| `slug` | The URL, `/<slug>`. |
| `order` | Position within its category (lower first; absent sorts last). |
| `description` | One-line summary under the `<h1>`. |
| `symptoms` | Extra keywords the index search matches. |
| `common` | `true` pins it into the "Common issues" shortlist. |
| `notice` | A banner above the body. |
| `vars` | Runtime inputs — see the next step. |

## Inputs with `vars`

`vars` render a panel of inputs. Each value substitutes into code blocks as a `{{TOKEN}}`, where the token is uppercase A–Z and underscore only:

```bash [Use the inputs]
ssh "{{HOST}}"
systemctl --user restart "{{SERVICE}}"
```

- Give the diagnostic lookup to a `hint` rather than a literal `<placeholder>` in the code — angle brackets inside a fence look like real syntax.
- Set `secret: true` to render a password field.
- Substitution happens in the browser, so values **never leave the page**.

## Steps, subheadings and prose

`## Heading` starts a numbered step. `### Heading` splits a long step without starting a new card. A step body takes ordinary Markdown: **bold**, *italic*, `inline code`, [links](https://example.com), bullet lists and tables.

> [!info] Titles lead with the symptom, not the mechanism — a runbook is read under duress.

## Code blocks and checkoffs

A fenced block takes a lowercase language tag and an optional bracketed label. The label renders a header with a per-block **done** checkbox, and a step ticks itself once all its blocks are ticked.

```sql [Inspect the target]
SELECT id, state FROM information_schema.processlist WHERE user = '{{SERVICE}}';
```

## Notices and decisions

Three notice variants carry different weight:

> [!info] Context, links and read-only remarks.

> [!warn] A step that briefly stops writes, or a value to double-check.

> [!danger] Destructive. READ EVERY STEP BEFORE DOING ANYTHING.

A branch callout lists the decisions a reader might make:

> [!branch]
> - If the check passed, continue.
> - If it failed, rebuild instead.
> - If neither, page the on-call.

## Rollback

`---rollback` on its own line closes the numbered steps and starts a separate,
danger-styled, unnumbered section. This page ends with one, so you can see the
styling:

```text [The marker]
---rollback
```

That is the whole format — delete this file, and any other content you don't need,
from `content/` when you are done.

---rollback

## Undo the example restart

Rollback steps are unnumbered and danger-styled. A real runbook puts the actual undo
commands here.

```bash [Stop the example service]
systemctl --user stop "{{SERVICE}}"
```
