# Runbooks

A small Go web app that renders operational runbooks as interactive step-by-step
pages. Runbooks are plain Markdown files with YAML frontmatter — **no database, no
CMS**. One page per runbook, served by slug.

- **Repo:** `github.com/ladydascalie/runbooks` (private), local `~/Code/Personal/runbooks`.
- **Licence:** FSL-1.1-MIT (source-available, no competing use, converts to MIT after
  2y). Governance, positioning and the commercial model live in the spec library:
  `~/openspec/plans/specs/runbooks-governance.md`, `…/runbooks-commercial-model.md`.
- This file layers on the global `~/.pi/agent/AGENTS.md` (DB naming, API versioning, no
  foreign keys, memory/spec stores). There is no workspace parent above it.

## Commands

All tasks are mise tasks (`mise install` once to pull Go, templ, gofumpt, Node).

| Task | What it does |
|------|--------------|
| `mise run dev` | Live-reload server at <http://localhost:8091>. templ watch + esbuild watch for CSS/JS + `air`. |
| `mise run build` | `templ generate` → `go run ./cmd/css` → `go run ./cmd/js` → `go build -o runbooks .` |
| `mise run generate` | templ codegen only (`views/*_templ.go`). |
| `mise run css` / `mise run js` | One-shot esbuild bundle. |
| `mise run lintcss` / `mise run fmtcss` | stylelint / Prettier over `public/css/src/**`. |
| `mise run test` | Go tests + JS unit tests (`node --test`). |
| `mise run notices` | Regenerate `THIRD_PARTY_NOTICES.md` from the linked modules and vendored assets (`cmd/notices`). |
| `mise run sbom` / `mise run sbom:image` | SPDX 2.3 + CycloneDX SBOM for the built binary / the container image (syft). See `docs/sbom.md`. |
| `mise run test:e2e` | Browser E2E behind `e2e/harness.mjs` (shared boot + Chromium + virtual authenticator): `passkey.test.mjs` (identity), `runbook.test.mjs` (page interactions), `styleguide.test.mjs` (design-system screenshots). `E2E_SCREENSHOT_DIR=…` writes the screenshots. Needs a system Chromium (`CHROMIUM=/path` to override); `playwright-core` installs into `e2e/` on demand. |
| `mise run test:e2e:headed` | The same tests in visible windows. `E2E_SLOWMO=ms` paces the actions, `E2E_HOLD_MS=ms` holds at the end of each test (default: run straight through). |
| `mise run test:e2e:inspect` | Headed with the **Playwright Inspector** (`page.pause()` breakpoints before the setup, invite-enrol and recovery submits). Step over / resume from the inspector window. |

The verification gate before handing work back is `gofumpt -l .`, `go vet ./...`,
`mise run test`, and `mise run build`. The store contract also runs live against
compose MySQL/Postgres (`mise run test:mysql` / `test:postgres`).

Ports: dev serves `8091` (air proxies `8090` → `8091`). The binary reads `PORT`,
default `8090`; the container listens on `8090`.

## Design system

The app has its own visual identity — see `~/openspec/plans/specs/runbooks-design-system.md`.
The living reference is `/styleguide` (dev-gated by `STYLEGUIDE_ENABLED`, set in
`mise.toml` `[env]`), organised by atomicity; `/styleguide/llms` is the agent mirror
(embedded from `styleguide.llms.txt`).

- **Palette** — seeded from the mallard, dark-first (`:root` is dark;
  `html[data-theme="light"]` overrides). Roles, never raw colour; status is never the
  brand accent. See `tokens.css`.
- **Fonts** — self-hosted Atkinson Hyperlegible Next + Mono (variable, latin).
  `--mono-weight` is 450, or 600 under `html[data-code-weight="bold"]`.
- **Radii** — squared (4/6/8px); tickboxes are square, not circles.
- **Badges** — bracketed `[ label ]`, not filled pills.
- **Code surfaces stay dark in both themes** — use the constant `--code-*` tokens,
  never a theme role inside a code block (it renders dark-on-dark in light).
- **Contextual chrome belongs to the context**, not the component (e.g. the sidebar
  footer divider is `.sidebar .appearance`), so a component can be shown elsewhere
  without doubling a host frame.
- **CSS modules** live in `public/css/src/`, one subject per file: `fonts`, `tokens`,
  `typography`, `layouts`, `button`, `badge`, `alert`, `forms`, `dialog`, `shell`,
  `vars`, `tooltip`, `notes`, `tabs`, `toast`, `appearance`, `runbook`, `code`,
  `notice`, `table`, `card`, `index`, `auth`, `admin`, `responsive`, `styleguide`.
  `shell.css` is the chrome grid + sidebar/nav/main; `runbook.css` is the runbook
  page; `responsive.css` holds the below-1200px rail/drawers and the print block, and
  imports last so its overrides land after the components they target.

## Layout

```
main.go              loads content/, wires routes (index, runbook, admin, styleguide, auth, git-sync), serves /public/
auth.go              identity HTTP: setup/login/invite/recovery, session cookie, read gating, admin invite + revoke
styleguide.go        dev-gated /styleguide + /styleguide/llms
identity/            identity service: WebAuthn ceremonies, sessions, invites (no HTTP)
stores/              identity persistence: Store contract + sqlite/ mysql/ postgres/ + storetest/
parser/parser.go     frontmatter + body parser; content discovery; sidebar grouping/ordering
views/base.templ     Shell + shared components (SidebarNav, VarsPanel, CodeBlock, Step, Notice, AppearanceControl, SessionActions, …)
views/index.templ    welcome page at / (search + common-issue shortcuts + card catalogue)
views/runbook.templ  runbook page (steps, vars panel, notes panel)
views/admin.templ    /admin — invite form + users table
views/styleguide.templ  the design system at /styleguide
views/auth.templ     /login, /setup, /invite/<token>, /recovery
views/helpers.go     inline-markdown → HTML helpers, slugify, JSON embedding, PageConfig, theme script
content/<system>/<category>/<file>.md   the runbooks themselves
public/css/src/      source CSS (bundled → public/css/bundle.css)
public/fonts/        self-hosted Atkinson Hyperlegible Next + Mono (variable, latin)
public/js/src/       source JS (bundled → public/js/bundle.js)
public/js/vendor/    marked, highlight.js, jszip (checked in, not bundled)
e2e/                 browser E2E: harness.mjs + passkey/runbook/styleguide suites
cmd/css, cmd/js      esbuild wrappers (see build pipeline below)
cmd/notices          generates THIRD_PARTY_NOTICES.md (mise run notices)
LICENSE / NOTICE     outbound FSL-1.1-MIT + the third-party notices that must be embedded (OFL, MPL driver)
DEPENDENCIES.md      dependency inventory + FSL classification
THIRD_PARTY_NOTICES.md  generated full licence texts (do not hand-edit)
docs/sbom.md         SBOM formats, regeneration and verification
docs/layout.md       the layout contract: primitives, tokens, stylelint enforcement
tmp/                 air build output
```

## Content model

The **filesystem is the taxonomy**. `content/<system>/<category>/<file>.md`:

- `<system>` → top-level sidebar group (directory name, title-cased).
- `<category>` → subheading under the system.
- `content/<system>/<file>.md` (one level) → under the system, no subheading.
- Discovery is **recursive**: a runbook's system and category are the two directories
  directly above it, so a leading wrapper is ignored — `content/testdata/mysql/backup/
  x.md` groups as MySQL › Backup just like `content/mysql/backup/x.md`.
- The **slug comes from frontmatter**, not the filename; the URL is `/<slug>`.

A directory's **display name and sidebar order** come from an optional
`_meta.yaml` beside its content:

```yaml
# content/mysql/_meta.yaml
title: MySQL   # else the directory name is title-cased (disaster-recovery → Disaster Recovery)
order: 1       # lower first; absent sorts after ordered names, alphabetically
```

Both the system and the category may carry one. Nothing about naming or ordering
lives in Go — adding or reordering a system/category is a content edit only.
Within a category, runbooks sort by the optional frontmatter `order:` (lower first;
absent = `defaultRunbookOrder` = 100), then title.

The index page (`views.IndexPage`, route `/`) is data-driven too: `common: true`
pins a runbook into the "Common issues" shortlist (label = first symptom, via
`parser.CommonIssues`), while `symptoms:` is extra keyword coverage for the live
filter on every runbook. The filter matches title + description + symptoms +
system/category, by phrase first and then "all words present". There is no
separate index file to keep in sync.

### Frontmatter

```yaml
---
title: Replication Lag (MTS Deadlock)   # shown in sidebar + <h1>
slug: mts-deadlock-recovery              # URL: /<slug>
order: 1                                 # optional, within-category position
description: One-line summary shown under the <h1>.
symptoms:                                # optional: extra keywords the index filter matches
  - Last_SQL_Error
  - duplicate entry error 1062
common: true                             # optional: pin into the "Common issues" shortlist
notice: "READ EVERY STEP BEFORE DOING ANYTHING!"   # passive banner
acknowledge: "This drops and rebuilds the replica."  # gates the page behind an "I understand" dialog (destructive)
vars:
  - id: mts-processlist-id               # DOM id; must be unique
    label: Blocked worker's processlist Id
    var: PROCESSLIST_ID                  # token name; [A-Z_]+ only
    placeholder: "e.g. 4123"
    secret: true                         # renders a password input
    hint: |-                             # plain text popover, no markdown
      Where to find the value
---
```

`vars` are runtime inputs. Values are substituted into code blocks client-side via
`{{VAR_NAME}}`; **they never leave the page** (no network). The token regex is
`[A-Z_]+`, so lowercase or dashes will not substitute.

### Body syntax

- `## Heading` → a numbered step. `### Heading` → subheading inside a step. Steps
  are **collapsible** — the header toggles one, the toolbar has Expand all /
  Collapse all, and a collapsed step shows a one-line preview. The runbook's
  **Contents** list is **optional and off by default** (the collapsed steps are the
  contents); `runbooks-steps` (`first`|`all`) and `runbooks-contents` (`on`|`off`)
  are reading preferences in the Appearance control, applied before paint in
  `app.js`. **Zen** (a toolbar button, Esc to exit) hides the chrome and shows one
  step at a time.
- ` ```lang [Label] ` → code block; the bracket label renders a header with a
  per-block "done" checkbox. `lang` must match the parser's fence regex
  (`[a-z]*` — lowercase only).
- `> [!info]` / `> [!warn]` / `> [!danger]` → notices.
- `> [!branch]` + `> - item` → decision callout.
- `- item` lists, `| a | b |` tables.
- `---rollback` on its own line → everything after it is a separate rollback
  section (danger-styled, unnumbered).

Authoring conventions that have bitten us:

- **Titles lead with the symptom**, not the internal mechanism (e.g.
  "Replication Lag (MTS Deadlock)", not "MTS Deadlock Recovery"). This is a
  runbook read under duress.
- Put the diagnostic lookup in a `vars` entry with a `hint` rather than a literal
  `<placeholder>` in the code — `<foo>` in a fence looks like real syntax.
- Use `order:` to pull the most-likely-on-call runbook to the top of its category.

## Build pipeline and generated files

- **templ**: edit `views/*.templ`. `views/*_templ.go` is generated and **gitignored** —
  never hand-edit it; it is rebuilt by `mise run build` and by CI/Docker
  (`go tool templ generate ./...`), so run `mise run generate` after changing a
  `.templ`.
- **CSS**: `public/css/src/main.css` `@import`s `fonts`, `tokens`, `layouts`,
  `shell`, `runbooks`. esbuild bundles/minifies to `public/css/bundle.css`
  (committed). Put component styles in the relevant src file; the `.nav-system`
  heading lives in `shell.css`.
- **JS**: `public/js/src/app.js` → `public/js/bundle.js` (IIFE, committed).
  The no-flash theme script is inlined in `views/base.templ` from the
  `themeScript` const in `views/helpers.go`. Vanilla JS, no framework.
  Client behaviour: var substitution + copy, step/block completion persisted in the
  URL hash, the notes completion timeline, hint popovers, notes panel (localStorage,
  image paste, ZIP export, sync), notes resize, theme switcher.
- **Embedding**: `main.go` `//go:embed public` embeds *assets only*. **`content/`
  is read from disk at runtime** (`parser.LoadDir("content")`), so content edits
  need no rebuild — and the container `COPY`s `content/` alongside the binary.

## Gotchas

- **`content/` is private operational material.** It describes real infrastructure
  procedures and must not be published. Until the app is separated from this
  content, keep the repo private — the app itself is source-available, the *content*
  is not for release.
- **CI verifies generated files are committed and current**: `check.yml` fails if
  `public/css/bundle.css`, `public/js/bundle.js` or `THIRD_PARTY_NOTICES.md` differ
  from a fresh regeneration. Run the matching task (`mise run build` / `mise run notices`)
  and commit the result.
- **Build outputs are gitignored** (`/runbooks`, `/tmp/`). `mise run build`
  produces the binary, `air` writes `tmp/`. Never commit them.
- **`content/` is empty in a fresh checkout.** That is fine — the index renders a
  welcome page. Add a runbook under `content/<system>/<category>/<file>.md` to populate it.
- `mise run build` also rewrites `public/css/bundle.css` and `public/js/*.js`.
  Those are committed on purpose; include them in the same change as their src.
- `/` serves the welcome/index page (`views.IndexPage`), not a redirect;
  unknown paths 404. The index is generated from `parser.CommonIssues` +
  `GroupBySystem`, so it needs no content file of its own.
- stylelint: `declaration-no-important` is on (two documented exceptions in
  `.stylelintrc.json` — `code.css` over the syntax highlighter, `responsive.css` the
  override layer), and `declaration-property-unit-allowed-list` bans raw px in
  `margin`/`padding`/`gap` — spacing comes from tokens (use `--space-hair` below
  `--space-1`). See `docs/layout.md`. Prettier uses tabs, width 100.
- **E2E virtual-authenticator options**: Chromium's `WebAuthn.addVirtualAuthenticator`
  names the backup flags `defaultBackupEligibility` / `defaultBackupState`; the
  `hasBackup*` spellings are silently ignored (so BE never gets set). `e2e/passkey.test.mjs`
  relies on the `default*` names to present a synced-passkey (BE=1) shape.
- **Below 1200px the shell reflows** (thin rail + off-canvas drawers, driven by `data-drawer` on `<body>`; see `responsive.css`): the 196px sidebar would otherwise starve the main column on portrait/thin screens. `app.js` drives the rail menu, notes toggle, scrim and Esc.
- The runbook page's right-hand notes panel is hideable; the toggle (`data-notes-toggle`)
  sits in the `.main-header` and persists `runbooks-notes` (on|off, default on),
  applied before paint in `app.js`.
- The notes **Sync** button only renders when the server has git sync enabled
  (`GITSYNC_REPO` + a credential + endpoint auth). With identity on, `/api/git-sync/v1`
  requires a session and the commit is authored as the signed-in user
  (`DisplayName <Email>`); `GITSYNC_API_TOKEN` is the CI/automation fallback and
  `GITSYNC_AUTHOR_*` applies only when the user has no email. See the git-sync spec
  for the config; the dev config lives in `mise.toml` `[env]`.

## Licence & SBOM

- `LICENSE` is the FSL-1.1-MIT text; `DEPENDENCIES.md` inventories every distributed
  dependency and its licence; `NOTICE` embeds the OFL text for the fonts and the
  MPL-2.0 notice for `go-sql-driver/mysql` (compatible via MPL §3.3 Larger Work —
  see `specs/runbooks-commercial-model.md`).
- `THIRD_PARTY_NOTICES.md` is **generated** (`mise run notices`, `cmd/notices`) —
  never hand-edit it; CI fails on drift. The JS and font licence texts are committed
  beside the assets.
- `mise run sbom` / `sbom:image` emit SPDX 2.3 + CycloneDX (`docs/sbom.md`); CI
  uploads them as the `sbom` artifact and, on `v*` tags, keyless-attests the SPDX
  document with cosign.

## Deploy

- CI `.github/workflows/container.yml` builds on PRs and pushes on `main` to
  `ghcr.io/ladydascalie/runbooks:<branch>-<utc-datetime>-<sha>`, plus `…:latest` on
  `main` only. It generates SPDX + CycloneDX SBOMs for the image, uploads them as
  the `sbom` artifact, and on `v*` tags keyless-attests the SPDX document with cosign.
- Deployment is not yet defined for this personal repo (the previous ArgoCD/gitops
  wiring belonged to the old owner and was dropped with the move).

## Working rules for this repo

- **Do not commit or push without explicit human approval.** Never force-push
  `main`; never merge PRs.
- Keep changes to the app small and unabstracted: plain functions in `parser` and
  `views/helpers.go`, no new layers. `views` owns presentation; `parser` owns the
  content contract.
