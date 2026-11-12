# Exception: forward-only migrations 000002, 000015, and 000016 rewritten in place for session and audit short IDs

- Profile and requirement: architecture invariant that migrations are
  forward-only ([architecture](../../architecture.md#data-and-scaling)); applied
  migration 000002 is rewritten in place to change `sessions.id` and
  `audit_events.id` from `TEXT` to integer keys, and applied migrations 000015
  and 000016 are rewritten in place to change `mfa_challenges.session_id` and
  `oauth_grants.session_id` from `TEXT` to `BIGINT`.
- Status: proposed
- Responsible owner: repository maintainers (issue #243, ticket #248)
- Approving maintainer: pending. The repository owner must approve this record
  before it is marked accepted.
- Security-owner approval, when required: not required; no security control is
  weakened. The session cookie value stays the secret, and lookups stay on
  `secret_hash`. A row's integer key or short ID is never accepted as the
  session secret.
- Created: 2026-11-12
- Review by: 2027-02-10
- Expires: 2027-02-10

## Reason and evidence

Ticket #248 moves `sessions` and `audit_events` to integer keys with the
`session` and `audit_event` short IDs, and converts the two session foreign keys
to match. Sessions are exposed in the session list, the revoke route, and
session audit targets. Audit events are exported with their short IDs.

Both new keys use `{{file_id_pk}}`, which is `INTEGER PRIMARY KEY AUTOINCREMENT`
on SQLite. The parent decision for this ticket is that a deleted maximum key
must never be reused when the key is referenced by audit or exposed as an ID.
`TestSQLiteSessionAndAuditKeysAreNeverReused` checks that on SQLite. PostgreSQL
identities never reuse a key, so the PostgreSQL definition is unchanged.

The rewrite follows the reset approach of the earlier exceptions
([2026-11-12-oauth-tokens-short-id-migration-rewrite](2026-11-12-oauth-tokens-short-id-migration-rewrite.md)
and [2026-11-12-mfa-short-id-migration-rewrite](2026-11-12-mfa-short-id-migration-rewrite.md)).
A forward migration would leave `TEXT` keys in the historical schema and need a
data conversion for session and audit rows. Verification is in
[capabilities](../../capabilities.md#session-and-audit-short-id-evidence).

## User, security, compatibility, and operational risk

- Any database that already applied migrations 000002, 000015, or 000016 keeps
  the old `TEXT` keys and records the versions as applied. The new build does
  not reapply them and fails on integer keys. Such databases cannot be upgraded
  in place and must be recreated.
- Sessions, audit history, MFA challenges, and OAuth grants that exist when the
  database is recreated are lost. Users must sign in again, and active MCP grants
  must be re-consented.
- The session list, revoke route, and session audit targets change from random
  32-hex IDs to `session` short IDs. The revoke route returns the same
  invalid-session problem for a malformed, foreign, integer, or unknown ID.
- Audit `target_id` values for session targets and exported audit event IDs
  change to short IDs. Consumers that stored the old form must map it.
- OIDC link flows bind their transaction to `session.ID` (`browser_session_hash`
  stores the hash of that value). The ID is now a short ID instead of a random
  hex value. The callback still requires the session cookie, which stays secret,
  so a guessable short ID does not let another browser complete the link. The
  owner should confirm this reading.

## Compensating controls

- `TestSQLiteSessionAndAuditTablesUseIntegerPrimaryKeys` and
  `TestPostgresSessionAndAuditTablesUseIntegerPrimaryKeys` check the key types on
  both databases.
- `TestSQLiteSessionAndAuditKeysAreNeverReused` checks that a deleted maximum
  session key and audit key are not reused on SQLite.
- `TestSessionShortIDsAreExposedAndNeverAccepted` checks that the short ID is
  listed and audited, that the row key and decimal key are not accepted as the
  secret or as a CSRF session, that malformed, integer, and foreign-type IDs
  revoke nothing, and that the owner's own short ID revokes.
- `TestIdentityRESTContracts` checks that a malformed revoke and an
  already-revoked revoke return the same invalid-session problem.
- The operations guide states that this version must not run against an earlier
  migrated database ([operations](../../operations.md#upgrades-and-alerts)).

## Replacement or removal path

Before any release that has deployed instances, a forward migration path is
required if in-place conversion is needed. Otherwise, remove this exception once
the release notes state the reset requirement. Review on or before 2027-02-10.

## Verification

- Verification results are recorded in
  [capabilities](../../capabilities.md#session-and-audit-short-id-evidence).
