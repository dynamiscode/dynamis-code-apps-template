# Exception: forward-only migration rewritten for notification and account token short IDs

- Profile and requirement: architecture invariant that migrations are
  forward-only ([architecture](../../architecture.md#data-and-scaling)); applied
  migration 000007 is rewritten in place for the `notifications`,
  `email_verifications`, and `password_resets` primary keys.
- Status: accepted
- Responsible owner: repository maintainers (spec #243, ticket #245)
- Approving maintainer: repository maintainer, approved in spec #243 ("implement
  as written"), which requires integer keys for these tables
- Security-owner approval, when required: not required; no security control is
  weakened. Email verification and password reset secrets stay random and hashed
  as before, and lookups stay on `token_hash`. A row's integer key or short ID is
  never accepted as the secret.
- Created: 2026-11-12
- Review by: 2027-02-10
- Expires: 2027-02-10

## Reason and evidence

Ticket #245 requires `notifications`, `email_verifications`, and `password_resets`
to use integer primary keys (`INTEGER PRIMARY KEY` on SQLite,
`BIGINT GENERATED ALWAYS AS IDENTITY` on PostgreSQL). Notification IDs in the
web JSON, SSE events, and the `after` cursor expose only the ADR 0009 short ID.
Email verification and password reset rows keep no exposed ID. A forward
migration would leave `TEXT` keys in the historical schema and add conversions
across three tables. The change follows the reset approach of the earlier
exceptions, including
[2026-11-11-sharing-scim-oauth-short-id-migration-rewrite](2026-11-11-sharing-scim-oauth-short-id-migration-rewrite.md).
Verification is in [capabilities](../../capabilities.md#notification-and-account-token-short-id-evidence).

## User, security, compatibility, and operational risk

- Any database that already applied version 000007 keeps the old `TEXT` keys and
  records that version as applied. The new build does not reapply it and fails
  on integer keys. Such databases cannot be upgraded in place. Deployed
  instances, if any exist, must be recreated or restored into a new database. No
  automatic conversion is provided.
- Pending email verification and password reset links are not carried into a
  recreated database. Users request a new link after the reset.
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
- Tests prove that a verification or reset row's integer key, and its short ID,
  fail as secrets, while the emailed secret still succeeds.

## Replacement or removal path

Before any release that has deployed instances, a forward migration path is
required if in-place conversion is needed. Otherwise, remove this exception once
the release notes state the reset requirement. Review on or before 2027-02-10.

## Verification

- Verification results are recorded in
  [capabilities](../../capabilities.md#notification-and-account-token-short-id-evidence).
