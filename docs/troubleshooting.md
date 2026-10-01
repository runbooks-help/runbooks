# Troubleshooting

## Colours look washed out / low contrast (HDR displays)

On an **HDR display**, some **Chromium-based browsers** (Chrome, Brave, Edge) on
Linux convert standard-dynamic-range (SDR) page content into the HDR output
incorrectly: the blacks are lifted and the dark theme looks flat and washed out,
with no true black. **Firefox renders it correctly** on the same display.

There is **no app-side fix**: Runbooks is fixed sRGB, and declaring it standard-dynamic
range (`dynamic-range-limit: standard`) does not change it. It is a browser/compositor
bug, so the workarounds are on the reader's side:

- **Disable HDR** while you work (KDE: *System Settings → Display & Monitor → HDR*).
- **Use Firefox**, which renders it correctly, or
- **Lower the SDR brightness** in your display settings — a value near peak (e.g.
  ~400 nits) lifts SDR blacks the most; ~100–200 nits is closer to how SDR is meant
  to look.

What you see in Firefox (or with HDR off) is the intended rendering; nothing in a
runbook changes it.
