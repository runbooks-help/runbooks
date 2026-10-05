# Security policy

## Reporting a vulnerability

Please report suspected vulnerabilities **privately**, not in a public issue.

- **Preferred:** GitHub's private vulnerability reporting: open the repository's
  **Security** tab and choose **Report a vulnerability**. This opens a private
  advisory that only the maintainers can see.
- **Alternative:** email [contact@runbooks.help](mailto:contact@runbooks.help).
- If neither works, open a minimal public issue asking a maintainer to make
  contact, and do not include the details.

A useful report includes what you found, the affected version or commit, the
impact, and a reproduction if you have one.

## Scope

This policy covers the Runbooks application in this repository: the Go server,
its passkey identity and read-scoped agent surfaces, and the published container
image. It does not cover an operator's own content or deployment, nor
third-party dependencies (report those upstream; see `DEPENDENCIES.md`).

## Supported versions

Runbooks is source-available and maintained on a best-effort basis. Security
fixes land on `main` and in the latest tagged release; older releases are not
maintained.

## What to expect

This is a maintainer-led project with no security team and no bug-bounty
programme. Reports are acknowledged and investigated as time allows; there is no
guaranteed response time. Credit is given on request once a fix ships. Thank you
for reporting responsibly.
