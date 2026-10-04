# Dependencies, licences and attribution

Audit of every dependency that ships with Runbooks, classified against the
project's outbound licence, **FSL-1.1-MIT** (Functional Source License 1.1, MIT
future licence). Spec: `runbooks-commercial-model.md`; governance:
`runbooks-governance.md`.

- Date: 2026-10-03
- Scope: what actually ships — the Go binary, the vendored browser JS and
  icons, the self-hosted fonts, and the container base. Build/test-only tooling
  is recorded too, because it affects CI and reproducibility.
- Posture: FSL is source-available, not OSI-open — it carries a two-year
  competing-use restriction, then converts to MIT. **Copyleft dependencies cannot
  be combined with it** (their terms forbid adding that further restriction), so
  every distributed dependency must be permissive (MIT / BSD / Apache-2.0 / ISC)
  or font-OFL, or be resolved.

## Result

**No distributed dependency is incompatible.** One is weak copyleft and is
resolved by the Larger Work clause — see the note below. Nothing needs replacing
or rewriting.

| Class | Count | Licences |
|---|---|---|
| Distributed Go modules (linked into the binary) | 46 | Apache-2.0 (8), BSD-2-Clause (2), BSD-3-Clause (19), MIT (16), **MPL-2.0 (1)** |
| Vendored browser JS | 4 | BSD-3-Clause, MIT, MIT-or-GPLv3 (elect MIT), Apache-2.0 (DOMPurify) |
| Vendored icons | 1 | ISC (Feather-derived subset MIT) |
| Self-hosted fonts | 2 | OFL-1.1 |
| Build/test-only Go modules | 59 | MIT, BSD-2/3-Clause, Apache-2.0, ISC, MPL-2.0 (1, not linked) |
| Build tooling (non-Go) | — | MIT, Apache-2.0 |

### The one weak-copyleft dependency

`github.com/go-sql-driver/mysql` (v1.10.1) is **MPL-2.0**, a *file-level* copyleft.
It is compatible with shipping a product under FSL:

- MPL 2.0 §3.3 ("Distribution of a Larger Work") lets us distribute the combined
  binary under terms of our choice (FSL), provided we meet the MPL conditions for
  the driver itself.
- Those conditions: keep the driver under MPL-2.0, keep its notices, and — when
  distributing the executable — make the driver's source available and do not let
  our terms restrict the recipient's rights in the driver's source (§3.1, §3.2).
  The driver is **unmodified**, so pointing at upstream satisfies source
  availability; we ship the MPL text in `NOTICE`.
- We do **not** assert FSL over the driver; the `NOTICE` carves it out explicitly.

MySQL is not droppable (it is the most common database), and there is no
permissive Go driver with equal capability and weight: the MIT alternative
(`go-mysql-org/go-mysql/driver`) pulls a heavy module graph, and MariaDB's
first-party connectors are LGPL-2.1 and not available for Go. Keeping the MPL
driver is the correct call.

## 1. Distributed Go modules

Linked into the `runbooks` binary (from `go list -deps .`). Every one is
permissive except the noted MPL driver. The git transport is part of this set:
`go-git` (Apache-2.0) and its tree — `go-git/go-billy`, `ProtonMail/go-crypto`,
`skeema/knownhosts`, `xanzy/ssh-agent`, `pjbgf/sha1cd`, `filepath-securejoin`, …
— replaced shelling out to a system `git` binary, so neither the container nor a
bare-binary self-host needs git installed.

| Module | Version | Licence |
|---|---|---|
| `dario.cat/mergo` | v1.0.0 | BSD-3-Clause |
| `filippo.io/edwards25519` | v1.2.0 | BSD-3-Clause |
| `github.com/a-h/templ` | v0.3.1020 | MIT |
| `github.com/cloudflare/circl` | v1.6.3 | BSD-3-Clause |
| `github.com/cyphar/filepath-securejoin` | v0.6.1 | BSD-3-Clause |
| `github.com/dustin/go-humanize` | v1.0.1 | MIT |
| `github.com/emirpasic/gods` | v1.18.1 | BSD-2-Clause |
| `github.com/fxamacker/cbor/v2` | v2.9.4 | MIT |
| `github.com/go-git/gcfg` | v1.5.1-0.20230307220236-3a3c6141e376 | BSD-3-Clause |
| `github.com/go-git/go-billy/v5` | v5.9.0 | Apache-2.0 |
| `github.com/go-git/go-git/v5` | v5.19.2 | Apache-2.0 |
| `github.com/golang/groupcache` | v0.0.0-20241129210726-2c02b8208cf8 | Apache-2.0 |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | MIT |
| `github.com/google/go-tpm` | v0.9.8 | Apache-2.0 |
| `github.com/google/uuid` | v1.6.0 | BSD-3-Clause |
| `github.com/go-sql-driver/mysql` | v1.10.1 | **MPL-2.0** (Larger Work, §3.3) |
| `github.com/go-viper/mapstructure/v2` | v2.5.0 | MIT |
| `github.com/go-webauthn/webauthn` | v0.18.2 | BSD-3-Clause |
| `github.com/go-webauthn/x` | v0.3.1 | BSD-3-Clause |
| `github.com/jackc/pgpassfile` | v1.0.0 | MIT |
| `github.com/jackc/pgservicefile` | v0.0.0-20240606120523-5a60cdf6a761 | MIT |
| `github.com/jackc/pgx/v5` | v5.11.0 | MIT |
| `github.com/jackc/puddle/v2` | v2.2.2 | MIT |
| `github.com/jbenet/go-context` | v0.0.0-20150711004518-d14ea06fba99 | MIT |
| `github.com/kevinburke/ssh_config` | v1.2.0 | MIT |
| `github.com/klauspost/cpuid/v2` | v2.3.0 | MIT |
| `github.com/philhofer/fwd` | v1.2.0 | MIT |
| `github.com/pjbgf/sha1cd` | v0.6.0 | Apache-2.0 |
| `github.com/ProtonMail/go-crypto` | v1.1.6 | BSD-3-Clause |
| `github.com/remyoudompheng/bigfft` | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause |
| `github.com/sergi/go-diff` | v1.3.2-0.20230802210424-5b0b94c5c0d3 | MIT |
| `github.com/skeema/knownhosts` | v1.3.1 | Apache-2.0 |
| `github.com/tinylib/msgp` | v1.6.4 | MIT |
| `github.com/x448/float16` | v0.8.4 | MIT |
| `github.com/xanzy/ssh-agent` | v0.3.3 | Apache-2.0 |
| `golang.org/x/crypto` | v0.57.0 | BSD-3-Clause |
| `golang.org/x/net` | v0.59.0 | BSD-3-Clause |
| `golang.org/x/sync` | v0.23.0 | BSD-3-Clause |
| `golang.org/x/sys` | v0.48.0 | BSD-3-Clause |
| `golang.org/x/text` | v0.42.0 | BSD-3-Clause |
| `gopkg.in/warnings.v0` | v0.1.2 | BSD-2-Clause |
| `gopkg.in/yaml.v3` | v3.0.1 | Apache-2.0 (or MIT) |
| `modernc.org/libc` | v1.77.1 | BSD-3-Clause (+ third-party MIT / public domain) |
| `modernc.org/mathutil` | v1.7.1 | BSD-3-Clause |
| `modernc.org/memory` | v1.12.1 | BSD-3-Clause |
| `modernc.org/sqlite` | v1.60.1 | BSD-3-Clause; SQLite public domain; sqlite_vec MIT |

Source for each: `https://pkg.go.dev/<module>@<version>`.

## 2. Vendored browser JS (`public/js/vendor/`)

Checked in and served as-is; not bundled.

| File | Version | Licence | Elected |
|---|---|---|---|
| `highlight.min.js` | 11.11.1 | BSD-3-Clause | — |
| `marked.min.js` | 15.0.12 | MIT | — |
| `purify.min.js` | 3.2.4 | Apache-2.0 **or** MPL-2.0 | **Apache-2.0** |
| `jszip.min.js` | 3.10.1 | MIT **or** GPLv3 | **MIT** (bundled pako is MIT) |

## 3. Vendored icons (`views/`)

Server-rendered inline SVG — a subset of [Lucide](https://lucide.dev) 1.49.0,
ISC-licensed, with some icons derived from Feather (MIT). The path data is
compiled into the binary via `views/icons/icons.go`; the licence text is
`public/js/vendor/lucide.LICENSE` and ships in `THIRD_PARTY_NOTICES.md`. No icon
runtime is distributed.

| Icons | Version | Licence |
|---|---|---|
| bold, italic, code, link, image, list, list-ordered, heading, quote, circle-help, ellipsis | 1.49.0 | ISC (Feather subset MIT) |

## 4. Fonts (`public/fonts/`)

| File | Font | Licence |
|---|---|---|
| `atkinson-hyperlegible-next-latin-wght-normal.woff2` | Atkinson Hyperlegible Next | OFL-1.1 |
| `atkinson-hyperlegible-mono-latin-wght-normal.woff2` | Atkinson Hyperlegible Mono | OFL-1.1 |

Both are © Braille Institute of America, licensed under the SIL Open Font
License 1.1. The OFL text must accompany the fonts — see `NOTICE`.

## 5. Build/test-only Go modules

Present in `go.mod` (direct or indirect) but **not linked into the shipped
binary**. Recorded for CI and reproducibility; all are permissive except
`hashicorp/golang-lru/v2` (MPL-2.0, unused at runtime and not distributed).

| Module | Version | Licence |
|---|---|---|
| `github.com/a-h/parse` | v0.0.0-20250122154542-74294addb73e | MIT |
| `github.com/andybalholm/brotli` | v1.1.0 | MIT |
| `github.com/cenkalti/backoff/v4` | v4.3.0 | MIT |
| `github.com/cli/browser` | v1.3.0 | BSD-2-Clause |
| `github.com/creack/pty` | v1.1.9 | MIT |
| `github.com/davecgh/go-spew` | v1.1.1 | ISC |
| `github.com/descope/virtualwebauthn` | v1.0.5 | MIT |
| `github.com/evanw/esbuild` | v0.28.0 | MIT |
| `github.com/fatih/color` | v1.16.0 | MIT |
| `github.com/fsnotify/fsnotify` | v1.7.0 | BSD-3-Clause |
| `github.com/fxamacker/webauthn` | v0.6.1 | Apache-2.0 |
| `github.com/google/go-cmp` | v0.7.0 | BSD-3-Clause |
| `github.com/google/go-tpm-tools` | v0.3.13-0.20230620182252-4639ecce2aba | Apache-2.0 |
| `github.com/google/pprof` | v0.0.0-20260802141513-ef3492d7dac3 | Apache-2.0 |
| `github.com/hashicorp/golang-lru/v2` | v2.0.7 | MPL-2.0 |
| `github.com/kr/pretty` | v0.3.0 | MIT |
| `github.com/kr/text` | v0.2.0 | MIT |
| `github.com/mattn/go-colorable` | v0.1.13 | MIT |
| `github.com/mattn/go-isatty` | v0.0.24 | MIT |
| `github.com/natefinch/atomic` | v1.0.1 | MIT |
| `github.com/ncruces/go-strftime` | v1.0.0 | MIT |
| `github.com/pmezard/go-difflib` | v1.0.0 | BSD-2-Clause |
| `github.com/rogpeppe/go-internal` | v1.16.0 | BSD-3-Clause |
| `github.com/rs/cors` | v1.11.0 | MIT |
| `github.com/stretchr/objx` | v0.1.0 | MIT |
| `github.com/stretchr/testify` | v1.12.1 | MIT |
| `github.com/yuin/goldmark` | v1.4.13 | MIT |
| `go.uber.org/mock` | v0.6.0 | Apache-2.0 |
| `go.yaml.in/yaml/v3` | v3.0.5 | MIT OR Apache-2.0 |
| `golang.org/x/mod` | v0.41.0 | BSD-3-Clause |
| `golang.org/x/net` | v0.59.0 | BSD-3-Clause |
| `golang.org/x/telemetry` | v0.0.0-20260908163034-4bcc4b2ee518 | BSD-3-Clause |
| `golang.org/x/term` | v0.46.0 | BSD-3-Clause |
| `golang.org/x/tools` | v0.50.0 | BSD-3-Clause |
| `gopkg.in/check.v1` | v1.0.0-20201130134442-10cb98267c6c | BSD-2-Clause |
| `modernc.org/cc/v4` | v4.29.7 | BSD-3-Clause |
| `modernc.org/ccgo/v4` | v4.36.1 | BSD-3-Clause |
| `modernc.org/fileutil` | v1.4.0 | BSD-3-Clause |
| `modernc.org/gc/v2` | v2.6.5 | BSD-3-Clause |
| `modernc.org/gc/v3` | v3.1.5 | BSD-3-Clause |
| `modernc.org/goabi0` | v0.2.0 | BSD-3-Clause |
| `modernc.org/opt` | v0.2.0 | BSD-3-Clause |
| `modernc.org/sortutil` | v1.2.1 | BSD-3-Clause |
| `modernc.org/strutil` | v1.2.1 | BSD-3-Clause |
| `modernc.org/token` | v1.1.0 | BSD-3-Clause |

## 6. Build tooling (non-Go)

Not distributed, but part of the build/CI story.

| Tool | Licence | Role |
|---|---|---|
| templ (`go tool templ`) | MIT | codegen for `views/*.templ` |
| esbuild (`go run ./cmd/js`, `./cmd/css`) | MIT | bundles `public/js`, `public/css` |
| stylelint / Prettier (npm) | MIT | CSS lint / format |
| playwright-core (npm, `e2e/`) | Apache-2.0 | browser E2E |

## 7. Container

The runtime image is stock `alpine:3` plus `ca-certificates` from Alpine's
package index — the go-git transport removed the `git` and `openssh-client`
packages the image used to carry. Alpine packages carry their own licences
(mostly MIT / BSD / GPL where Alpine permits it) and are the base image's
responsibility, not linked into `runbooks`; per-package licence metadata is
available with `apk info -L <pkg>`. The chain therefore reads: the `runbooks`
binary (FSL-1.1-MIT, with `NOTICE` + `THIRD_PARTY_NOTICES.md`), plus the Alpine
base image (its own licences). The image ships `LICENSE`, `NOTICE`,
`DEPENDENCIES.md` and `THIRD_PARTY_NOTICES.md` alongside the binary.

## Regenerating this inventory

```bash
go list -m all                                     # every module in the graph
go list -deps -f '{{with .Module}}{{.Path}}{{end}}' . | sort -u   # linked into the binary
```

A machine-readable SBOM (SPDX + CycloneDX, covering the binary and the container
image) is generated by `mise run sbom` / `mise run sbom:image`; see `docs/internals/sbom.md`.
This file is its human-readable companion.

## Third-party licence texts

`THIRD_PARTY_NOTICES.md` reproduces the full licence text and copyright for every
distributed component. It is generated — do not edit it by hand:

```bash
mise run notices
```

CI regenerates it and fails if the committed file differs.

## Source-file headers

Every source file Runbooks authors carries `SPDX-License-Identifier: FSL-1.1-MIT`
at the top. `mise run lint:spdx` verifies it (CI runs the same check); apply
missing headers with `go run ./cmd/spdx -fix`. Vendored third-party assets,
generated files and the Markdown docs are out of scope.
