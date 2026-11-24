---
name: change-go-data
description: Change Dynamis Code database schemas, ordered migrations, queries, repositories, transactions, or data lifecycle while preserving SQLite and PostgreSQL compatibility and workspace isolation.
---

# Change Go Data

Read [AGENTS.md](../../../AGENTS.md),
[architecture](../../../docs/architecture.md),
[data lifecycle](../../../docs/data-lifecycle.md),
[operations](../../../docs/operations.md), and
[capabilities](../../../docs/capabilities.md). Inspect every caller of the
repository or data contract being changed.

## Outcome

Produce one forward-safe data change that behaves consistently on real SQLite
and PostgreSQL databases and cannot cross workspace scope.

## Workflow

Before parallel schema work, the coordinator fetches the target branch and
records its commit and highest migration version, then assigns each worker a
distinct version and intended merge order. Share the allocation with every
worker before migrations are written. Include any other in-flight schema work;
an allocation is coordination, not a reservation enforced by Git.

```sh
git fetch origin
git rev-parse origin/main
git ls-tree --name-only origin/main internal/platform/database/migrations/
```

Workers start from that base and report their migration name and any new schema
dependency to the coordinator. Before committing a migration, refresh the base
and allocations; if the target or another worker has claimed its version,
coordinate a new unused version and merge order before committing. Check the
working tree for changes to existing migrations. Before opening or updating a
PR, fetch again and run the existing history guard against the latest target:

```sh
git status --short -- internal/platform/database/migrations/
git diff --name-status origin/main -- internal/platform/database/migrations/
sh scripts/check-migration-history.sh origin/main HEAD
```

The guard checks committed history; inspect the working tree separately. After
another schema PR lands, repeat the preflight and verify the intended order and
dependencies. Do not rename, renumber, or edit a committed migration to resolve
a collision: the history guard rejects retained renames, and an applied version
must remain immutable. Coordinate a safe forward migration or replacement branch
after confirming whether the migration was applied; never rewrite an applied
migration.

1. Define ownership, classification, purpose, retention, export, deletion, UTC
   representation, identifier, foreign-key, and index behavior for each field.
2. Add one ordered migration history for both databases. Use vendor-specific
   SQL only with a documented compatibility reason and fallback.
3. Pass workspace identifiers explicitly. Filter workspace-owned reads and
   mutations by workspace plus resource identifier.
4. Keep multi-operation transactions in the application use case. Avoid an
   ORM, generic repository, implicit global scope, and SQL-string mocks.
5. For rollout-sensitive changes, use expand-contract or document an explicit
   stop-the-world upgrade, interruption behavior, rollback limit, and restore
   path.
6. Update migration, repository, lifecycle, operations, and capability
   evidence together.

## Verify and stop

Run focused use-case tests, real temporary SQLite tests, isolated PostgreSQL
migration/repository tests, then the available Go gates. Stop if either
database differs in domain behavior, a destructive migration lacks verified
recovery, or workspace scoping is inferred rather than explicit.
