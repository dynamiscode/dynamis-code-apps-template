# Exception: forward-only migrations rewritten for webhook and job short IDs

- Profile and requirement: architecture invariant that migrations are
  forward-only ([architecture](../../architecture.md#data-and-scaling)). Applied
  migrations 000008 (webhooks), 000010 (background jobs), and 000013
  (merge compatibility, including the `background_jobs` backfill) are rewritten
  in place. The same range was already rewritten by the
  [workspace exception](2026-11-11-workspace-short-id-migration-rewrite.md).
- Status: proposed. Not approved. Until a maintainer records approval here, this
  rewrite is non-conforming.
- Responsible owner: repository maintainers (issue #192)
- Approving maintainer: pending. The implementation request for #192 did not
  name a migration approach, so no approval is claimed.
- Security-owner approval, when required: not required; no security control is
  weakened, and webhook authorization, secret encryption, and signing are
  unchanged.
- Created: 2026-11-11
- Review by: 2027-02-09
- Expires: 2027-02-09

## Reason and evidence

Issue #192 requires webhook and background-job records to take integer keys
and short-ID type codes. The `webhooks`, `webhook_deliveries`, and
`background_jobs` primary keys were `TEXT`. A forward migration would have to
rebuild three tables and convert every delivery and job reference, which is the
same cost the workspace exception avoided. This change adds `webhook_events`,
so each event has a short ID for the `eventId` response field and the
`Webhook-Id` header. Verification: see
[capabilities](../../capabilities.md#webhook-and-job-short-id-evidence).

## User, security, compatibility, and operational risk

- Any database that already applied versions 000008, 000010, or 000013 keeps
  the text-keyed tables. The new build does not reapply them and fails on
  integer keys. Such databases cannot be upgraded in place.
- Pending webhook deliveries and jobs are not carried across the reset. A
  deployment that must keep them needs a drain before the reset.
- Outbound `Webhook-Id` values change from 32-character hex to short IDs.
  Receivers that deduplicate by that header see new values after the upgrade.
  Deduplication still works within one version of the stream.
- Authorization, audit semantics, signing, and secret handling are unchanged.

## Compensating controls

- The operations guide states that this version must not run against an earlier
  migrated database ([operations](../../operations.md#upgrades-and-alerts)).
- The migration backfill is covered by an SQLite migration test that seeds
  integer keys, and the full suite runs on SQLite and on a fresh pinned
  PostgreSQL 14.24 container.

## Replacement or removal path

Before any release with deployed instances, a forward migration path is
required if in-place conversion is needed. Otherwise, remove this exception once
the release notes state the reset requirement. Review on or before 2027-02-09.
