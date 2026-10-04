---
title: Brand assets
slug: brand
order: 7
layout: sections
description: Regenerating the logomark, wordmark and lockups via mise run brand.
---

`mise run brand` generates the brand assets into `brand/` from `cmd/brand`. The
output is committed, so the task is a manual step, not part of `mise run build`
or CI.

## What it writes

For each variant, an outlined SVG plus 512 and 1024 PNGs:

| Variant | `brand/` files |
|---|---|
| Logomark — `░▓` alone | `logomark.svg`, `logomark-{512,1024}.png` |
| Wordmark — `runbooks` | `wordmark-on-{dark,light}.svg`, `…-{512,1024}.png` |
| Horizontal lockup — mark + wordmark | `lockup-horizontal-on-{dark,light}.svg`, `…-{512,1024}.png` |
| Stacked lockup — mark above wordmark | `lockup-stacked-on-{dark,light}.svg`, `…-{512,1024}.png` |
| Icon — mark on the brand green | `icon.svg`, `icon-{512,1024}.png`, and `public/favicon.svg` |

The SVGs have their text converted to paths, so they render with no font
installed — the point of shipping SVG. `on-dark` means "for a dark background"
(the ink is light); `on-light` is the reverse. The logomark and icon use the
dark-theme brand greens on a transparent background and need no theme variant.

The task also writes `public/favicon.svg` from the **icon**, so the served
favicon cannot drift from the generated mark.

## How it works

`cmd/brand` writes a source SVG per variant with the self-hosted fonts, then
shells out to the image toolchain already on the machine:

1. `woff2_decompress` turns the vendored `public/fonts/*.woff2` into TTFs in a
   temp dir, and a temp fontconfig file points only at them, so the outliner
   uses the pinned fonts and not whatever is installed system-wide.
2. `inkscape --export-text-to-path --export-area-drawing` outlines the text and
   crops to the drawing, writing `brand/<name>.svg`.
3. `rsvg-convert -w <n>` rasterises the outlined SVG to PNG.

So a run needs `woff2_decompress`, `inkscape` and `rsvg-convert` on `PATH`
(Inkscape and librsvg are the Arch packages `inkscape` and `librsvg`). No
browser.

The palette is copied from `public/css/src/tokens.css`; see `cmd/brand/main.go`.

## The shade font

The `░`/`▓` glyphs (and the bookshelf's `─│┌┐└┘├┤` frame) are not in Atkinson
Hyperlegible, so the app used to render the mark, the bookshelf and its frame
through the OS fallback monospace — Noto Sans Mono here, DejaVu or Menlo
elsewhere — which made the logo machine-dependent and left the shelves a
half-character short. `public/fonts/noto-sans-mono-shades.woff2` is a **glyph
subset** of Noto Sans Mono (box drawing + shade blocks, U+2500–U+2593), pinned
under the family name `Shade Mono` and declared in `public/css/src/fonts.css`.
The mark (`.book-mark`) and the whole bookcase (`.shelf-grid`, so every cell and
the 13ch track measure `ch` in one face) use it via the `--font-shade` token.

Rebuild it from a Noto Sans Mono Regular TTF (OFL) with:

```sh
pyftsubset NotoSansMono-Regular.ttf --unicodes=U+2500-2593 \
  --output-file=shade.ttf --no-hinting --desubroutinize \
  --layout-features='' --recalc-bounds
# then rename the family to "Shade Mono" (name IDs 1/3/4/6/16) and
woff2_compress shade.ttf   # → noto-sans-mono-shades.woff2
```

Its licence ships beside it at `public/fonts/noto-sans-mono-shades.LICENSE`;
`mise run notices` reproduces it in `THIRD_PARTY_NOTICES.md`.
