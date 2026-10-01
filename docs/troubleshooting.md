# Troubleshooting

## Colours look washed out / low contrast (HDR displays)

On an **HDR display**, some **Chromium-based browsers** (Chrome, Brave, Edge) on
Linux convert standard-dynamic-range (SDR) page content into the HDR output
incorrectly: the blacks are lifted and the dark theme looks flat and washed out,
with no true black. **Firefox renders it correctly** on the same display.

Runbooks is a fixed-sRGB, SDR interface and declares `dynamic-range-limit: standard`
to ask the browser not to do this. If it still looks wrong:

- **Disable HDR** while you work (KDE: *System Settings → Display & Monitor → HDR*).
- **Use Firefox**, or
- **Lower the SDR brightness** in your display settings — a value near peak (e.g.
  ~400 nits) lifts SDR blacks the most; ~100–200 nits is closer to how SDR is meant
  to look.

This is a browser/compositor bug, not the app: the palette is fixed sRGB hex, so
what you see in Firefox (or with HDR off) is the intended rendering. There is
nothing to change in a runbook to avoid it.
