# Exception: forward-only migration 000015 rewritten for MFA short IDs

- Profile and requirement: architecture invariant that migrations are
  forward-only ([architecture](../../architecture.md#data-and-scaling)); applied
  migration 000015 is rewritten in place. Its `mfa_challenges`,
  `mfa_recovery_codes`, and `mfa_passkeys` primary keys change from `TEXT` to
  integer keys. The `sessions` columns in the same file are unchanged.
- Status: accepted
- Responsible owner: repository maintainers (spec #243, ticket #246)
- Approving maintainer: repository owner, through spec #243, which directs the
  in-place rewrite for each short-ID table
- Security-owner approval, when required: not required; no security control is
  weakened. Challenge tokens and recovery codes stay random and hashed
  (`token_hash`, `code_hash`), and a row ID is never accepted as either.
  Passkey `credential_id` stays text because it is a WebAuthn protocol
  identifier.
- Created: 2026-11-12
- Review by: 2027-02-10
- Expires: 2027-02-10

## Reason and evidence

Spec #243 requires MFA challenges, recovery codes, and passkeys to use integer
primary keys (`INTEGER PRIMARY KEY` on SQLite, `BIGINT GENERATED ALWAYS AS
IDENTITY` on PostgreSQL) and to expose only their ADR 0009 short IDs under the
`mfa_challenge`, `mfa_recovery_code`, and `mfa_passkey` type codes. No other
table references these three tables, so the rewrite touches only them. It
follows the reset approach of the workspace exception
([2026-11-11-workspace-short-id-migration-rewrite](2026-11-11-workspace-short-id-migration-rewrite.md)).
Verification is in [capabilities](../../capabilities.md#mfa-short-id-evidence).

## User, security, compatibility, and operational risk

- Any database that already applied version 000015 keeps the old `TEXT` keys and
  records that version as applied. The new build does not reapply it, and the
  MFA paths fail on the old schema. Such databases cannot be upgraded in place.
  The version-15 compatibility check in the migration runner only applies when
  version 15 is not yet recorded, so it does not cover this case.
- Deployed instances, if any exist, must be recreated or restored into a new
  database. No automatic conversion is provided.
- `mfa_passkeys.credential_id` stays text, and `user_id` and
  `mfa_challenges.session_id` stay text until their own tickets (#248, #249)
  convert them.
- Passkey IDs in list and removal responses change from random hex to short IDs.
  Removal decodes the short ID strictly after password verification, and a
  malformed ID returns the same error as a missing passkey. Audit target IDs
  for MFA challenges and passkeys carry the short ID.

## Compensating controls

- The operations guide states that this version must not run against an earlier
  migrated database ([operations](../../operations.md#upgrades-and-alerts)).
- The migration runner renders the integer key per driver and is covered by a
  SQLite schema test and an isolated PostgreSQL schema test from a fresh
  database.
- The identity tests check that challenge and recovery-code row IDs, as integers
  or short IDs, are rejected as the bearer secret, and that the real code still
  works afterward.

## Replacement or removal path

Before any release that has deployed instances, a forward migration path is
required if in-place conversion is needed. Otherwise, remove this exception once
the release notes state the reset requirement. Review on or before 2027-02-10.

## Verification

- Verification results are recorded in
  [capabilities](../../capabilities.md#mfa-short-id-evidence).
