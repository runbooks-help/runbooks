# Troubleshooting

## Colours look washed out / low contrast (HDR displays)

On an **HDR display**, some **Chromium-based browsers** (Chrome, Brave, Edge) on
Linux/Wayland convert standard-dynamic-range (SDR) page content into the HDR output
incorrectly: the blacks are lifted and the dark theme looks flat and washed out,
with no true black. Firefox renders it correctly on the same display.

The cause is Chromium's use of the Wayland colour-management protocol
(`wp_color_manager_v1`) under a compositor HDR session. **Fix it in the browser** by
launching it with:

```
--disable-features=WaylandWpColorManagerV1
```

Add that flag to the launcher — the `Exec=` line of the browser's `.desktop` file,
or its flags file where the launcher supports one (`~/.config/brave-flags.conf`,
`~/.config/chromium-flags.conf`). The trade-off: the browser then no longer
colour-manages *HDR* content either, which is irrelevant for a text app but matters
if you use the same browser for HDR video.

Alternatives, if you would rather not set a flag:

- **Disable HDR** while you work (KDE: *System Settings → Display & Monitor → HDR*).
- **Use Firefox**, which has no Linux HDR output path and renders SDR as authored.
- **Lower the SDR brightness** in your display settings — a value near peak (e.g.
  ~400 nits) lifts SDR blacks the most; ~100–200 nits is closer to how SDR is meant
  to look.

There is no app-side fix: Runbooks is fixed sRGB hex, and `dynamic-range-limit:
standard` was tried and does not help. What you see in Firefox, with the flag, or
with HDR off is the intended rendering.
