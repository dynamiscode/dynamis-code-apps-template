# Exception: forward-only migrations rewritten in place for user and external identity short IDs

- Profile and requirement: architecture invariant that migrations are
  forward-only ([architecture](../../architecture.md#data-and-scaling)). Applied
  migrations 000002, 000003, 000005, 000007, 000012, 000013, 000014, 000015,
  000016, and 000020 are rewritten in place. `users.id` and
  `external_identities.id` change from `TEXT` to integer keys. Every text
  `users(id)` foreign key column in those migrations changes to `BIGINT`: 21
  columns across 16 tables. `oidc_transactions.state_hash` stays a hash-keyed
  `TEXT` primary key; this record also covers that decision.
- Status: proposed
- Responsible owner: repository maintainers (issue #243, ticket #249)
- Approving maintainer: pending. The repository owner must approve this record
  before it is marked accepted.
- Security-owner approval, when required: not required. No credential is
  weakened. A user's integer key or short ID is never accepted as a credential.
  `oidc_transactions.state_hash` and every other secret lookup stay on hash
  columns.
- Created: 2026-11-12
- Review by: 2027-02-10
- Expires: 2027-02-10

## Reason and evidence

Ticket #249 gives users and external identities integer keys with the `user`
and `external_identity` short IDs. Every text user foreign key moves to the same
integer type, so no user reference is left as text. `users` and
`external_identities` use `{{file_id_pk}}`, which is `INTEGER PRIMARY KEY
AUTOINCREMENT` on SQLite and `BIGINT GENERATED ALWAYS AS IDENTITY` on
PostgreSQL. This follows the parent decision that a deleted maximum key is never
reused when it is referenced or exposed.
`TestSQLiteUserAndExternalIdentityKeysAreNeverReused` checks that on SQLite.

Other decisions made in this ticket:

- `audit_events.actor_user_id` stays `TEXT`. It is not a foreign key, and the
  ticket does not list it. Audit rows now store the actor's user short ID. The
  integer key is never written there.
- `scim_users.external_id` stays protocol text. The default external ID for a
  newly provisioned user is that user's short ID. The migration backfill
  (`CAST(user_id AS TEXT)`) writes the decimal key, but it only copies rows that
  already exist when the migration runs. Fresh databases have none, and any
  database that applied the earlier history is recreated.
- `oidc_transactions.state_hash` stays the hash-keyed primary key. The
  transaction's `user_id` foreign key converts to `BIGINT` in migration 000005.
- MFA and WebAuthn user handles now use the user's short ID, not the old 32-hex
  value. Passkeys registered before the reset would not match, which is covered
  by the recreation requirement below.

The rewrite follows the reset approach of the earlier exceptions
([2026-11-12-session-audit-short-id-migration-rewrite](2026-11-12-session-audit-short-id-migration-rewrite.md)
and [2026-11-12-mfa-short-id-migration-rewrite](2026-11-12-mfa-short-id-migration-rewrite.md)).
A forward migration would leave `TEXT` keys in the historical schema and need a
data conversion for user, identity, and every foreign-key row.

## User, security, compatibility, and operational risk

- Any database that already applied migrations 000002 through 000020 keeps the
  old `TEXT` keys and records the versions as applied. The new build does not
  reapply them and fails on integer keys. Such databases cannot be upgraded in
  place and must be recreated.
- Users, identities, sessions, passkeys, tokens, and all other user-owned rows
  that exist when a database is recreated are lost. Users must sign in again and
  re-enroll passkeys.
- REST and MCP responses that contain user IDs change from 32-hex strings to
  `user` short IDs. The REST contract already declares these fields as strings,
  so the OpenAPI document does not change. MCP `createdByUserId` stays a string
  in its published schema.
- Login, OIDC, invitation, and token flows keep their behavior. Only the
  identifier they carry changes.

## Compensating controls

- `TestSQLiteUserTablesUseIntegerKeysAndForeignKeys` checks the integer key on
  `users` and `external_identities`, and the `BIGINT` type on each user foreign
  key on SQLite. Tables absent from a generated profile are skipped.
- `TestPostgresUserTablesUseIntegerKeysAndForeignKeys` checks the same types on
  PostgreSQL.
- `TestSQLiteUserAndExternalIdentityKeysAreNeverReused` checks that a deleted
  maximum user key and a deleted maximum external identity key are not reused on
  SQLite.
- `TestSessionShortIDsAreExposedAndNeverAccepted` and the session and audit tests
  from #248 still pass. Their audit actor check now binds the user short ID.
- The operations guide states that this version must not run against an earlier
  migrated database ([operations](../../operations.md#upgrades-and-alerts)).

## Replacement or removal path

Before any release that has deployed instances, a forward migration path is
required if in-place conversion is needed. Otherwise, remove this exception once
the release notes state the reset requirement. Review on or before 2027-02-10.

## Verification

- Verification results are recorded in
  [capabilities](../../capabilities.md#user-and-external-identity-short-id-evidence).
