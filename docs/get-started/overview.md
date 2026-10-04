---
title: Overview
slug: overview
order: 1
layout: sections
description: What Runbooks is, the mental model, and who it is for.
---

Runbooks is a small, self-hosted web app that turns a directory of Markdown files into interactive, step-by-step operational runbooks. **Prod broke? Runbooks help.**

## What it is

- **Files are the content.** A runbook is a Markdown file with YAML frontmatter. The directory path decides the sidebar. There is no database, no CMS and no authoring UI: you edit files in your editor and commit them.
- **One binary, one container.** The app reads a content directory at runtime, so content changes need no rebuild and no redeploy.
- **Procedures and documentation in one tool.** A `##` heading renders as a numbered, tickable, collapsible **step** (a procedure) or an unnumbered **section** (reference prose), and one page can mix both.
- **Built for reading under duress.** Symptom-first titles, collapsible steps, code blocks with a per-block "done" box, decision callouts, a rollback section, inline variables, notes, and a Zen mode that shows one step at a time.

## The mental model

| Thing             | How it works                                                               |
| ----------------- | -------------------------------------------------------------------------- |
| Taxonomy          | `content/<system>/<category>/<file>.md` → sidebar groups and subheadings   |
| URL               | the frontmatter `slug`, not the filename: `/<slug>`                        |
| Ordering          | `_meta.yml` at the content root, then per-file `order:`                    |
| Steps vs sections | a `##` heading; `layout: sections` or `---sections` / `---steps`           |
| Inputs            | frontmatter `vars`, substituted into code as `{{NAME}}`, client-side only |
| Search            | in-memory, full-body, served at `/api/runbooks/v1/search`                  |
| Agents            | `/llms.txt`, `/<slug>.md` (raw Markdown), and the search API               |

## Identity is optional

By default the app is public: no accounts, no database. Turn on passkey (WebAuthn) identity, or delegate to an upstream proxy/SSO gateway, to gate reads and attribute writes to named users. It is off until you configure it, and when it is on the instance is private (crawlers are disallowed).

## Who it is for

Solo operators, small teams and studios who keep their operational knowledge in files and want it readable in the moment. It is not aimed at large enterprises: there is no procurement process, no on-prem SSO tier and no enterprise sales motion.

## Licence

Runbooks is **source-available** under FSL-1.1-MIT: free to self-host, modify and use commercially, with no competing-use right, and each release converts to MIT two years after publication.

## Where next

- [Install & quickstart](install.md)
- [Writing a runbook](../guides/writing-runbooks.md)
