---
title: Component gallery (every runbook feature)
slug: gallery
order: 1
description: A single runbook that uses every frontmatter field and every body block, so the design system can be reviewed in one place.
symptoms:
  - component gallery
  - design system smoke test
  - every block type
common: true
notice: "Design fixture — nothing here is a real procedure. It exists to render every component."
vars:
  - id: gallery-host
    label: Target host
    var: HOST
    placeholder: "e.g. db-2.prod.internal"
    hint: |-
      Where the target lives.
      Use the read replica for dry runs, never the primary.
  - id: gallery-processlist-id
    label: Blocked worker's processlist Id
    var: PROCESSLIST_ID
    placeholder: "e.g. 4123"
    hint: As shown by information_schema.processlist.
  - id: gallery-api-token
    label: API token
    var: API_TOKEN
    secret: true
    hint: Rendered as a password field. Values never leave the page.
---

This opening paragraph is intro prose before the first step. It can carry **bold**, *italic*, `inline code`, and a [link](https://example.com) — all inline-markdown forms.

## Prose, lists and tables

A step body can hold plain prose, bullet lists and tables. The prose here is deliberately long enough to wrap a few lines, so line-height and measure are visible.

- First bullet — plain text.
- Second bullet with `inline code` and a **bold** run.
- Third bullet — a longer one that wraps onto a second line so spacing between items is visible.

| Column | Meaning | Example |
| --- | --- | --- |
| `HOST` | Target host | `db-2.prod.internal` |
| `PROCESSLIST_ID` | Blocked worker | `4123` |
| Status | Whether a step is done | `done` / `pending` |

### A subheading inside a step

Subheadings split a long step without starting a new numbered card, as above.

## Code blocks

Code blocks take a language and an optional bracketed label; the label renders a header with a per-block done checkbox. Variables substituted from the panel are highlighted.

```bash [Promote the replica]
export HOST="{{HOST}}"
mysql -h "$HOST" -e "STOP SLAVE; RESET SLAVE ALL;"
```

```sql [Inspect the worker]
SELECT id, time, state FROM information_schema.processlist WHERE id = {{PROCESSLIST_ID}};
```

```text [A label-less fence]
Any lower-case language tag is accepted; this fence has no bracketed label.
```

```bash [Uses the secret]
curl -H "Authorization: Bearer {{API_TOKEN}}" "https://{{HOST}}/healthz"
```

## Notices and branch

All three notice variants, then a decision callout.

> [!info] This is an info notice — context, links and read-only remarks.

> [!warn] This is a warning — a step that briefly stops writes, or a value to double-check.

> [!danger] This is a danger notice — destructive. READ EVERY STEP BEFORE DOING ANYTHING.

> [!branch]
> - If the replica's IO thread is running, continue to the next step.
> - If it has stopped, rebuild from the nightly snapshot instead.
> - If neither, page the on-call DBA.

---rollback

## Roll back the promotion

Rollback steps render after `---rollback`, in the danger styling and without a number.

```bash [Demote the node]
mysql -h "{{HOST}}" -e "SET GLOBAL read_only = ON;"
```
