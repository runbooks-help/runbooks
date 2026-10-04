---
title: Agent access
slug: agent-access
order: 3
layout: sections
description: Read-only access for LLM and automation clients, covering API keys, raw markdown and llms.txt.
---

Runbooks are plain Markdown with YAML frontmatter, which makes them easy for an LLM or an automation script to read. On a hosted instance they are also easy to expose through a small read-only surface rather than scraping the rendered HTML.

## The simplest path: clone the content directory

For a local agent, the best route needs no API: **clone the repo (or the `content/` directory) and point the agent at `content/`.** It gets the raw source (frontmatter included) and can grep across runbooks, follow cross-references, and read the git history for when a step changed and why. No network, no credentials, no parsed HTML.

`content/` mirrors the app: `content/<system>/<category>/<file>.md`, the slug in frontmatter, and the sidebar order in `content/_meta.yml`. An agent can read `_meta.yml` and each runbook's frontmatter directly.

There is deliberately **no `AGENTS.md` inside `content/`**: the parser treats every `*.md` under `content/` as a runbook, and a file without frontmatter is a startup error. Agent guidance lives here and in the repo's `AGENTS.md`.

## The hosted read API

When the agent runs remotely (a bot, a hosted chat, a different trust domain), it cannot clone anything. Instead the app exposes a **read-only** surface:

| Endpoint | What it returns |
| --- | --- |
| `GET /llms.txt` | A generated index: every runbook grouped by system, each linking to its `.md`. |
| `GET /<slug>.md` | One runbook's raw Markdown, frontmatter included. |
| `GET /api/runbooks/v1/search?q=…&limit=…` | Body search across titles, symptoms, steps and code; JSON with snippets and step anchors. |

With identity **off** these are public, like the rest of the app. With identity **on** they require either a session or a **read-scoped API key**, sent as:

```
Authorization: Bearer rbk_…
```

## Creating an API key

Keys are minted from your own account page and are read-only by construction.

<!-- steps -->

## Create the key

On an identity-enabled instance, open `/account` → **API keys**, give the key a label and choose **Create key**.

## Copy it once

The raw key (`rbk_…`) is shown **once**: copy it then; only its SHA-256 hash is stored and it cannot be recovered. Revoke a key from the same table.

<!-- sections -->

## What a key can do

A key is **read-only by construction**: it can call the three endpoints above and nothing else. It cannot view the HTML pages, reach the admin area, use git sync, or acknowledge a destructive runbook. A revoked key, or one whose owner has been disabled, stops working on the next request.
