# Governance

## License status

This checkout has no `LICENSE` file because the project has not selected a
license. Until maintainers make that decision and add the complete license
text at the repository root, this code is not offered under an open-source
license and no reuse permission should be inferred.

Before public distribution, maintainers must record:

1. the SPDX license identifier and copyright holder;
2. the complete corresponding text in `LICENSE`; and
3. any generated-application licensing rule in the release notes and template
   documentation.

Common candidates have different consequences and require maintainer/legal
review: MIT is permissive, Apache-2.0 adds an express patent license, and
AGPL-3.0 adds copyleft obligations for networked use. This repository does not
choose among them by default.

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

Release authority follows [release.md](release.md). A release must not imply
that the pending project license has been selected. Changes to license,
ownership, contribution rules, security reporting, or release authority need
maintainer approval and a changelog entry.

Dependency additions and upgrades must preserve the applicable upstream
license and notice terms. Update [NOTICE](../NOTICE), `go.mod`/`go.sum`, or
`package-lock.json` as applicable, and inspect the generated release SBOM.
