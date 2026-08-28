# Governance

## License

This project is licensed under MIT (`SPDX-License-Identifier: MIT`). The
copyright holder is David Londono; the complete license text is in
[`LICENSE`](../LICENSE).

`template-init` copies `LICENSE`, `NOTICE`, and the package SPDX metadata into
generated applications. It keeps the MIT terms and changes the copyright line
to the generated application name followed by `contributors`; application
maintainers must update that attribution for their actual contributors before
distribution.

## Maintainers

Maintainers own project direction, repository settings, dependency updates,
security response, releases, and changes to this governance document. The
default review owner is recorded in [CODEOWNERS](../.github/CODEOWNERS).

Maintainers should:

- keep changes small and reviewable;
- require the checks applicable to the changed surface;
- keep contracts, behavior documentation, capability evidence, and changelog
  entries aligned; and
- avoid committing credentials, personal data, authorization material, or
  signed URLs.

When a maintainer team exists, replace the individual CODEOWNERS entry with
the verified team and update branch protection to require its review.

## Contributions and review

Use the [contribution guide](../CONTRIBUTING.md), select the closest issue
template, and explain scope, verification, and intentional omissions in pull
requests. Security vulnerabilities belong in the private channel described by
[SECURITY.md](../SECURITY.md), never in a public issue.

Reviewers check behavior, authorization boundaries, data handling, accessible
fallbacks, dependency obligations, and documentation. A pull request may be
merged only after required checks pass and an owner approves it. The pull
request template is a checklist, not a substitute for review judgment.

## Releases and changes to governance

Release authority follows [release.md](release.md). Changes to license,
ownership, contribution rules, security reporting, or release authority need
maintainer approval and a changelog entry.

Dependency additions and upgrades must preserve the applicable upstream
license and notice terms. Update [NOTICE](../NOTICE), `go.mod`/`go.sum`, or
`package-lock.json` as applicable, and inspect the generated release SBOM.
