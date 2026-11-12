# Exception: forward-only migration 000015 rewritten for MFA passkey short IDs

- Profile and requirement: architecture invariant that migrations are
  forward-only ([architecture](../../architecture.md#data-and-scaling)); applied
  migration 000015 is rewritten in place. Its `mfa_passkeys` primary key changes
  from `TEXT` to an integer key. The `mfa_challenges` and `mfa_recovery_codes`
  keys, and the `sessions` columns in the same file, are unchanged.
- Status: accepted (approved by repository owner 2026-11-12; scope narrowed to
  `mfa_passkeys` only on 2026-11-12)
- Responsible owner: repository maintainers (spec #243, ticket #246)
- Approving maintainer: pending. The earlier approval in spec #243 covered
  `mfa_challenges` and `mfa_recovery_codes` as well; those keep their text keys
  and are not covered by this record.
- Security-owner approval, when required: not required; no security control is
  weakened. Passkey `credential_id` stays text because it is a WebAuthn protocol
  identifier, and a passkey row key is never accepted as a credential.
- Created: 2026-11-12
- Review by: 2027-02-10
- Expires: 2027-02-10

## Reason and evidence

Ticket #246 requires `mfa_passkeys` to use an integer primary key (`INTEGER PRIMARY KEY`
on SQLite, `BIGINT GENERATED ALWAYS AS IDENTITY` on PostgreSQL) and to expose only its
ADR 0009 short ID under the `mfa_passkey` type code in list and removal responses. No
other table references `mfa_passkeys`, so the rewrite touches only that table. It follows
the reset approach of the workspace exception
([2026-11-11-workspace-short-id-migration-rewrite](2026-11-11-workspace-short-id-migration-rewrite.md)).
Verification is in [capabilities](../../capabilities.md#mfa-passkey-short-id-evidence).

## User, security, compatibility, and operational risk

- Any database that already applied version 000015 keeps the old `TEXT` key for
  `mfa_passkeys` and records that version as applied. The new build does not reapply it, and the
  passkey paths fail on the old schema. Such databases cannot be upgraded in place.
  The version-15 compatibility check in the migration runner only applies when
  version 15 is not yet recorded, so it does not cover this case.
- Deployed instances, if any exist, must be recreated or restored into a new
  database. No automatic conversion is provided.
- `mfa_passkeys.credential_id` stays text, and `user_id` stays text until its own
  ticket converts it.
- Passkey IDs in list and removal responses change from random hex to short IDs.
  Removal decodes the short ID strictly after password verification, and a
  malformed ID returns the same error as a missing passkey. Audit target IDs
  for passkeys carry the short ID.

## Compensating controls

- The operations guide states that this version must not run against an earlier
  migrated database ([operations](../../operations.md#upgrades-and-alerts)).
- The migration runner renders the integer key per driver and is covered by a
  SQLite schema test and an isolated PostgreSQL schema test from a fresh
  database.
- The identity tests check that a malformed or foreign passkey ID is rejected
  with the same error as a missing passkey.

## Replacement or removal path

Before any release that has deployed instances, a forward migration path is
required if in-place conversion is needed. Otherwise, remove this exception once
the release notes state the reset requirement. Review on or before 2027-02-10.

## Verification

- Verification results are recorded in
  [capabilities](../../capabilities.md#mfa-passkey-short-id-evidence).
