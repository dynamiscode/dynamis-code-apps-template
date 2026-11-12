# Contributing

Read [AGENTS.md](AGENTS.md) and follow the task route in
[docs/README.md](docs/README.md). Keep each change independently reviewable;
preserve architecture boundaries and unrelated work.

Read [governance](docs/governance.md) for maintainer responsibilities, MIT
licensing, and dependency attribution. Use the repository issue and
pull-request templates for public work.

Before submitting:

```sh
make verify
make secret-check
make vuln-check
```

Run `npm ci && make accessibility-smoke`, PostgreSQL, container, restore, or
security checks when the
change touches those paths. Update generated contracts, migrations, behavior
docs, capability evidence, and `CHANGELOG.md` only when affected. Explain
failed, blocked, and not-applicable checks instead of weakening them. Enable
auto-merge (`gh pr merge <pr-number> --squash --delete-branch --auto`) to
sequence clean merges once CI passes, preserving all concurrent unreleased
`CHANGELOG.md` entries during rebases.

Dependabot opens weekly updates for Go, npm, Docker, and GitHub Actions. Minor
and patch updates are grouped per ecosystem into one PR, and CodeQL action
updates (including majors) are grouped into one PR; other major updates stay
separate for review.
Review dependency changes through CI; keep workflow actions pinned to full
commit SHAs with the release version in a comment. Do not merge an update that
introduces a vulnerability or generated drift.

Do not add dependencies or deferred capabilities without an accepted trigger
and decision. Report vulnerabilities through [SECURITY.md](SECURITY.md), not a
public issue.
