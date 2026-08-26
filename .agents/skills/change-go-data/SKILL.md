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
