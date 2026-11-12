# Exception: forward-only migration 000002 rewritten in place for audit event short IDs

- Profile and requirement: architecture invariant that migrations are
  forward-only ([architecture](../../architecture.md#data-and-scaling)); applied
  migration 000002 is rewritten in place to change `audit_events.id` from `TEXT`
  to an integer key. The `sessions` table in the same file is unchanged.
- Status: proposed (pending repository owner approval)
- Responsible owner: repository maintainers (issue #243, ticket #248)
- Approving maintainer: pending. The repository owner must approve this record
  before it is marked accepted.
- Security-owner approval, when required: not required; no security control is
  weakened. Audit event IDs are not credentials.
- Created: 2026-11-12
- Review by: 2027-02-10
- Expires: 2027-02-10

## Reason and evidence

Ticket #248 moves `audit_events` to an integer key with the `audit_event` short ID.
Audit event IDs are exported with their short IDs in workspace exports. The key uses
`{{file_id_pk}}`, which is `INTEGER PRIMARY KEY AUTOINCREMENT` on SQLite, so a deleted
maximum key is never reused. PostgreSQL identities never reuse a key, so the PostgreSQL
definition is unchanged in behavior.

The session table is not part of this record. Whether session IDs are element
identifiers is an open owner decision, so `sessions` keeps its `TEXT` key.

The rewrite follows the reset approach of the earlier exceptions, including
[2026-11-11-workspace-short-id-migration-rewrite](2026-11-11-workspace-short-id-migration-rewrite.md).
Verification is in [capabilities](../../capabilities.md#audit-event-short-id-evidence).

## User, security, compatibility, and operational risk

- Any database that already applied migration 000002 keeps the old `TEXT` key for
  `audit_events` and records the version as applied. The new build does not reapply
  it and fails on integer keys. Such databases cannot be upgraded in place and must
  be recreated. Audit history in a recreated database starts empty.
- Audit event IDs in workspace exports change from random hex to `audit_event`
  short IDs. Consumers that stored the old form must map it.

## Compensating controls

- The operations guide states that this version must not run against an earlier
  migrated database ([operations](../../operations.md#upgrades-and-alerts)).
- `TestSQLiteAuditTablesUseIntegerPrimaryKeys` and
  `TestPostgresAuditTablesUseIntegerPrimaryKeys` check the key type on both databases.
- `TestSQLiteAuditKeysAreNeverReused` checks that a deleted maximum audit key is not
  reused on SQLite.

## Replacement or removal path

Before any release that has deployed instances, a forward migration path is
required if in-place conversion is needed. Otherwise, remove this exception once
the release notes state the reset requirement. Review on or before 2027-02-10.

## Verification

- Verification results are recorded in
  [capabilities](../../capabilities.md#audit-event-short-id-evidence).
