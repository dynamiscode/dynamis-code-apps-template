# Exception: forward-only migration rewritten for notification short IDs

- Profile and requirement: architecture invariant that migrations are
  forward-only ([architecture](../../architecture.md#data-and-scaling)); applied
  migration 000007 is rewritten in place for the `notifications` primary key.
- Status: accepted (approved by repository owner 2026-11-12; scope narrowed to
  `notifications` only on 2026-11-12)
- Responsible owner: repository maintainers (spec #243, ticket #245)
- Approving maintainer: pending. The earlier approval in spec #243 covered
  `email_verifications` and `password_resets` as well; those tables keep their
  text keys and are not covered by this record.
- Security-owner approval, when required: not required; no security control is
  weakened. Notification IDs are not credentials.
- Created: 2026-11-12
- Review by: 2027-02-10
- Expires: 2027-02-10

## Reason and evidence

Ticket #245 requires `notifications` to use an integer primary key (`INTEGER PRIMARY KEY`
on SQLite, `BIGINT GENERATED ALWAYS AS IDENTITY` on PostgreSQL). Notification IDs in the
web JSON, SSE events, and the `after` cursor expose only the ADR 0009 short ID.
A forward migration would leave a `TEXT` key in the historical schema and add a
conversion. The change follows the reset approach of the earlier exceptions, including
[2026-11-11-sharing-scim-oauth-short-id-migration-rewrite](2026-11-11-sharing-scim-oauth-short-id-migration-rewrite.md).
Verification is in [capabilities](../../capabilities.md#notification-short-id-evidence).

## User, security, compatibility, and operational risk

- Any database that already applied version 000007 keeps the old `TEXT` key for
  `notifications` and records that version as applied. The new build does not reapply it and fails
  on integer keys. Such databases cannot be upgraded in place. Deployed
  instances, if any exist, must be recreated or restored into a new database. No
  automatic conversion is provided.
- Notification cursors held by an SSE client that predate this build are not
  valid short IDs. The server answers them with the existing `resync` event, so
  the client resumes from the latest notification.
- Authorization, audit semantics (notification audit targets now carry the short
  ID), and secret handling are otherwise unchanged. A malformed notification ID in
  mark-read returns the same denial as an unknown one.

## Compensating controls

- The operations guide states that this version must not run against an earlier
  migrated database ([operations](../../operations.md#upgrades-and-alerts)).
- The migration runner renders the one dialect-specific fragment per driver and
  is covered by SQLite and PostgreSQL tests from a fresh database.
- Tests prove that a malformed or integer notification cursor or mark-read ID is
  rejected, and that a valid short ID is accepted.

## Replacement or removal path

Before any release that has deployed instances, a forward migration path is
required if in-place conversion is needed. Otherwise, remove this exception once
the release notes state the reset requirement. Review on or before 2027-02-10.

## Verification

- Verification results are recorded in
  [capabilities](../../capabilities.md#notification-short-id-evidence).
