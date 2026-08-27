# Contributing

Read [AGENTS.md](AGENTS.md) and follow the task route in
[docs/README.md](docs/README.md). Keep each change independently reviewable;
preserve architecture boundaries and unrelated work.

Read [governance](docs/governance.md) for maintainer responsibilities,
dependency attribution, and the pending project-license decision. Use the
repository issue and pull-request templates for public work.

Before submitting:

```sh
make verify
```

Run `npm ci && make accessibility-smoke`, PostgreSQL, container, restore, or
security checks when the
change touches those paths. Update generated contracts, migrations, behavior
docs, capability evidence, and `CHANGELOG.md` only when affected. Explain
failed, blocked, and not-applicable checks instead of weakening them.

Do not add dependencies or deferred capabilities without an accepted trigger
and decision. Report vulnerabilities through [SECURITY.md](SECURITY.md), not a
public issue.
