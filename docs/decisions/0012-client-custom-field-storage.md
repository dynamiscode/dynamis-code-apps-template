# 0012: Client custom values use a JSON column with fixed index slots

- Status: accepted
- Date: 2026-11-13
- Owners: template maintainers

## Context

The Clients profile ([ADR 0013](0013-clients-profile.md)) lets each workspace
define custom fields on its clients. Custom values must filter and sort for
all five field types: text, number, date, yes/no, and single choice.

Constraints:

- Migrations are forward-only and are the only schema-change path
  ([architecture](../architecture.md)).
- Field names are not known at migration time, so a per-field index cannot be
  created by a migration.
- Creating an index at runtime is schema DDL outside the ordered migrations.
  On PostgreSQL, a plain `CREATE INDEX` blocks writes while it builds, and
  `CREATE INDEX CONCURRENTLY` cannot run inside a transaction.
- Filter and sort must work on SQLite and PostgreSQL, and every query stays
  scoped to one workspace ([ADR 0011](0011-identity-integer-keys.md)).

## Decision

- Each client row stores its custom values in one JSON object. Keys are field
  slots (`f01` through `f50`), not field names or IDs.
- Migrations create 50 fixed expression indexes per dialect, one per slot:
  SQLite on `json_extract`, PostgreSQL on jsonb expressions. Each index is
  scoped by workspace, then by the slot's value expression.
- A workspace has at most 50 custom fields, so each field takes one slot. A
  new field takes a free slot. Permanently deleting a field clears its values
  from every client and frees the slot in the same transaction.
- The configurable field limit defaults to 50 and cannot exceed the 50 slots.
- Clearing values is synchronous, in the request's transaction. There is no
  client-count cap in v1.
- Filter and sort queries are written for both dialects and checked on both
  (verification ladder step 7).

## Consequences

Benefits:

- No runtime DDL. Schema changes stay in ordered migrations.
- Filter and sort work for all five types, scoped per workspace.
- A client's custom values are written as one column in its own row.

Costs:

- Every filter and sort path has two SQL forms, one per dialect.
- The field limit is fixed at 50 by the slot count. Raising it needs a new
  migration that adds slots.
- Every write maintains all 50 expression indexes, whether or not a slot is in
  use. Measure write cost before raising the limit.
- Clearing a large workspace makes one delete request slow, with no ceiling in
  v1.

Rejected alternatives:

- Typed value table (one row per client and field, typed columns): a viable
  option, not chosen in design review.
- Per-field indexes created at runtime: needs runtime DDL and a recorded
  exception under `docs/decisions/exceptions/`.
- No indexes: filter and sort would scan every row, which fails the requirement
  to filter and sort on all types.
- Per-workspace generated columns or tables: per-tenant DDL that migrations
  manage poorly.

## Revisit when

- Measured write or query cost on a large workspace makes the 50 indexes too
  expensive.
- Workspaces need more than 50 custom fields.
- Synchronous clearing exceeds the request budget on a measured workspace. Move
  clearing to a background job ([ADR 0005](0005-background-jobs.md)).
