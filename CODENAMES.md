# Codenames

Each major release carries an alliterative adjective–animal name, Ubuntu-style, so
a release has a name to remember and the per-major clock is predictable (see
`specs/runbooks/commercial-model/README.md` § Cadence). Names are chosen at cut
time and are durable; the sequence is not alphabetical.

The release workflow reads the current major's name from the
`<!-- codename:… -->` marker below and titles the GitHub Release
`vX.Y.Z — <Codename>`. Add the marker when a new major is cut.

<!-- codename:v1 Meticulous Mallard -->

## v1 — Meticulous Mallard

- Released: 2026-10-05
- The first stable major: honour-mode self-hosting, the read-only agent surface
  and the git-backed content source.
