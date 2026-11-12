# Exception: forward-only migrations rewritten for workspace short IDs

- Profile and requirement: architecture invariant that migrations are
  forward-only ([architecture](../../architecture.md#data-and-scaling)); applied
  migrations 000002 through 000016 and 000018 are rewritten in place. Issue #190
  extends the same rewrite to item keys: `items.id` becomes an integer key, and
  `item_events.item_id` and `public_links.item_id` become integer references.
  Migration 000009 becomes an empty placeholder because its table rebuild
  copied explicit IDs, which PostgreSQL identity columns reject.
- Status: accepted
- Responsible owner: repository maintainers (issue #189)
- Approving maintainer: repository maintainer, approved in the implementation
  request for issue #189 ("implement as written") and, for the item-key
  extension in issue #190, in the user's approval of the #190 scope ("approve 190")
- Security-owner approval, when required: not required; no security control is
  weakened, and workspace authorization is unchanged.
- Created: 2026-11-11
- Review by: 2027-02-09
- Expires: 2027-02-09

## Reason and evidence

Issue #189 requires workspace primary keys to become integers (`INTEGER PRIMARY
KEY` on SQLite, `BIGINT GENERATED ALWAYS AS IDENTITY` on PostgreSQL) and every
`workspace_id` to become an integer foreign key. A forward migration would
leave the `TEXT` workspace keys in the historical schema and add a conversion
across sixteen tables. The issue accepts a reset of local development databases
instead. Verification: the full suite passes on SQLite and on a fresh pinned
PostgreSQL 14.24 container (see [capabilities](../../capabilities.md#workspace-short-id-evidence)).

Issue #193 extends the same in-place rewrite of migration 000002 to give
`workspace_members`, `invitations`, and `api_tokens` integer keys. Their
`(workspace_id, user_id)` membership key becomes a `UNIQUE` constraint. Existing
databases need the same reset described below.

## User, security, compatibility, and operational risk

- Any database that already applied versions 000002 through 000016 and 000018 keeps the old
  schema and records those versions as applied. The new build does not reapply
  them and fails on integer workspace keys. Such databases cannot be upgraded in
  place.
- Deployed instances, if any exist, must be recreated or restored into a new
  database. No automatic conversion is provided.
- Authorization, audit semantics, and secret handling are unchanged.

## Compensating controls

- The operations guide states that this version must not run against an earlier
  migrated database ([operations](../../operations.md#upgrades-and-alerts)).
- The migration runner renders the one dialect-specific fragment per driver and
  is covered by SQLite and PostgreSQL tests from a fresh database.

## Replacement or removal path

Before any release that has deployed instances, a forward migration path is
required if in-place conversion is needed. Otherwise, remove this exception once
the release notes state the reset requirement. Review on or before 2027-02-09.

## Verification

- Issue #190 item-key extension: `go test -count=1 ./...` passed on SQLite;
  `go test -p 1 -count=1` passed on a fresh PostgreSQL 14.24 container for the
  items, sharing, database, maintenance, identity, portability, and files
  packages. See [capabilities](../../capabilities.md#item-short-id-evidence).
- `go test -count=1 ./...` passed on SQLite.
- `POSTGRES_TEST_URL` set to a fresh PostgreSQL container, `go test -p 1 -count=1 ./...`
  passed.
- Restoring an earlier-built PostgreSQL database was not exercised.
