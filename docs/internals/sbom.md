---
title: SBOM
slug: sbom
order: 3
layout: sections
description: SPDX and CycloneDX bills of materials for the binary and the container image.
---

A machine-readable **Software Bill of Materials** for what Runbooks ships,
generated with [syft](https://github.com/anchore/syft) (Apache-2.0, pinned in
`mise.toml`). It complements the human-readable `DEPENDENCIES.md` and
`THIRD_PARTY_NOTICES.md`.

## Formats

Both are written on every run:

- `sbom/runbooks.spdx.json` — **SPDX 2.3 JSON** (ISO/IEC 5962).
- `sbom/runbooks.cdx.json` — **CycloneDX JSON**.

`sbom/` is gitignored: SBOMs are release artifacts, not source.

## What is covered

- **`mise run sbom`** — builds the binary and SBOMs it. The Go toolchain embeds
  the module list in the binary, so this captures every module linked into
  `runbooks` (the same set as `DEPENDENCIES.md` § 1).
- **`mise run sbom:image`** — builds the container image and SBOMs it. This is
  the actually-shipped artifact, so it adds the Alpine base and its apk packages
  (`git`, `openssh-client`, `ca-certificates`) on top of the binary's modules.

```bash
mise run sbom          # binary
mise run sbom:image    # container (builds the image first; needs podman)
```

CI generates both formats from the built image on every run and uploads them as
the `sbom` artifact. On a `v*` tag it also attaches both to the **GitHub
Release** for that tag, so a version's SBOMs sit with the release (and the tag)
rather than only in the Actions run.

## Verifying a release

On a tagged release CI signs the image and attests the SPDX document against it
with [cosign](https://github.com/sigstore/cosign), binding both to the image
digest. Verify the signature:

```bash
cosign verify \
  --certificate-identity-regexp '^https://github.com/ladydascalie/runbooks/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/ladydascalie/runbooks:<tag>
```

and the SBOM attestation:

```bash
cosign verify-attestation \
  --type spdxjson \
  --certificate-identity-regexp '^https://github.com/ladydascalie/runbooks/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/ladydascalie/runbooks:<tag>
```

## Limits

- syft reports the **declared** licences it can read; it is not a legal audit.
  The classification and the FSL posture live in `DEPENDENCIES.md` / `NOTICE`.
- Base-image apk packages are recorded but not re-derived — their licences are
  Alpine's responsibility.
- The SBOM describes build inputs, not a vulnerability assessment. Scanning
  (e.g. `grype`) and VEX are separate, not wired here.
