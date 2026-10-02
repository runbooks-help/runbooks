# Layout contract

How the runbooks UI is composed, and what enforces it. Read this before adding a
page or a component; the token definitions live in `public/css/src/tokens.css`.

## Shell

Every page renders inside the shell (`shell.css`): `<aside class="rail">` +
`<aside class="sidebar">` + `<div class="main">`, with a `.main-header` and a
scrolling `.content`. `.content` sets `scroll-padding-top` so anchored section links
do not land flush against the header.

The grid is `rail | main | notes`. The rail is the only persistent left chrome
(`--size-app-rail`): the brand mark plus a full-height hit area that summons the
sidebar. The sidebar is never a column — it overlays the rail and main at every
width, driven by `data-drawer="sidebar"` on `<body>` and closed on the scrim, Esc,
✕ or navigation. The brand keeps its own link home.

A page with no notes column marks its main `.main--full`, collapsing the grid to
`rail | main`. `body.notes-hidden` does the same for the reading preference, and
`body.zen` drops to a single centred column.

Below **1200px** (`responsive.css`) the notes panel becomes an off-canvas drawer
too — a 460px column starves main on a portrait monitor or in a split window. It is
the same `data-drawer` mechanism; `responsive.css` is the override layer, imports
last, and is the one place allowed to use `!important`.

## Primitives

Layout composes the primitives in `layouts.css`; a component does not hand-roll
`display: flex`/`grid`. They are shown live at `/styleguide#layouts`.

| primitive | what it is | options |
|---|---|---|
| `.stack` | vertical column | `data-gap="sm\|md\|lg"`, default `--space-4` |
| `.cluster` | wrapping row — chips, badges, actions | `data-gap="sm\|md\|lg"` |
| `.row` | non-wrapping row, `min-width: 0` baked in | `data-gap="sm\|md\|lg"` |
| `.inline-icon` | `inline-flex`, centred — an icon beside its label | — |
| `.grid` | CSS grid | `data-cols="2\|3\|4\|auto"`; `--grid-min` for `auto` |
| `.grow` | fill the remaining width in a `.row` | — |
| `.truncate` | `overflow: hidden` + nowrap + ellipsis | — |
| `.list-row` | `auto 1fr auto` — icon · content · action | — |
| `.detail-shell` | two-column `list \| detail` grid | `--size-detail-min`/`-max` |

Example — a header row with a status badge, a truncating title and a trailing
action:

```html
<div class="row" data-gap="md">
  <span class="badge badge-info">mysql</span>
  <span class="truncate">Replica lag on db-2 — the SQL thread is blocked</span>
  <button class="btn btn-ghost" type="button">Open</button>
</div>
```

## Tokens

Values live in `tokens.css`; components reference them, never a literal.

- **Spacing** — `--space-0..7` (0, 4, 8, 12, 16, 24, 32, 48) plus `--space-hair`
  (2px) for the sub-scale gaps and hairlines below `--space-1`.
- **Structural sizes** — `--size-app-rail`, `--size-app-sidebar`,
  `--size-app-header`, `--size-detail-min`/`-max`.
- **Component sizing** — `--ctrl-pad-y` sets interactive control height (button,
  input, select); `--size-step-num` is the step-number badge, reused by the
  collapsed-step indent so the two cannot drift.
- **Type** — `--font-sans` / `--font-mono`, `--mono-weight`, the `--text-2xs..2xl`
  scale (moved by `--text-scale`), `--leading-*`.
- **Radius / shadow / motion** — `--radius-sm|md|lg`, `--shadow-sm|md|lg`,
  `--duration-fast|base`, `--ease`.
- **Colour** — roles, never raw colour; status is never the brand accent; code
  surfaces stay dark in both themes via the constant `--code-*` tokens.

## Rules

1. **Compose primitives.** Page and component layout uses the primitives above, not
   bespoke flex/grid.
2. **Spacing comes from tokens.** No raw px in `margin*`, `padding*`, `gap`,
   `row-gap` or `column-gap` — this is machine-enforced (below). Use `--space-hair`
   when the value is genuinely sub-scale.
3. **Contextual chrome belongs to the context.** A reusable component carries no
   border or margin that only makes sense in one host — e.g. the sidebar footer
   divider is `.sidebar .appearance`, so the same component can be shown elsewhere
   without doubling a frame.
4. **Frontmatter `notice` is a banner; a notice is prose.** See the design-system
   spec for the alert/notice split.

## Enforcement

`mise run lintcss` runs stylelint over `public/css/src/**`. The spacing rule is:

```json
"declaration-property-unit-allowed-list": {
  "/^(margin|padding|gap|row-gap|column-gap)/": ["em", "rem", "%"]
}
```

It flags any `px` in those properties — including inside `calc()` — and allows
`var(--space-*)`, `0`, `auto` and px in non-spacing properties (widths, borders,
shadows, grid tracks). Probe it by adding `padding: 13px` to a source file: lintcss
fails.

`declaration-no-important` is on, with two documented exceptions in
`.stylelintrc.json`:

- `code.css` — the variable placeholder must win over the vendored syntax
  highlighter's token colours;
- `responsive.css` — the responsive and print override layer.

**Not machine-enforced** (review-only): "no bespoke flex/grid in a component" and
"roles, never raw colour". Catching either would mean parsing layout intent, not
syntax; the spec keeps them as review guidance.