# Exception: forward-only migration rewritten for item event short IDs

- Profile and requirement: architecture invariant that migrations are
  forward-only ([architecture](../../architecture.md#data-and-scaling)); applied
  migration 000004 (`item_events`) is rewritten in place. Its `id` changes from
  `TEXT` to an integer key. The table is rebuilt only by replacing its creation
  SQL, and no other migration changes.
- Status: accepted (approved by repository owner 2026-11-12)
- Responsible owner: repository maintainers (issue #244, spec #243)
- Approving maintainer: pending
- Security-owner approval, when required: not required; no security control is
  weakened. Change-feed authorization and workspace scoping are unchanged.
- Created: 2026-11-12
- Review by: 2027-02-09
- Expires: 2027-02-09

## Reason and evidence

Spec #243 requires every entity table to use an integer key and a short ID.
Migration 000004 is the only remaining text-keyed item table. A forward
migration would need to rebuild `item_events` and convert its IDs in place, which
costs more than the reset approach used by the earlier exceptions
([workspace](2026-11-11-workspace-short-id-migration-rewrite.md)). The table holds
at most 1,000 events per workspace and seven days of replay history, so a reset
loses only replay history. The key uses `AUTOINCREMENT` on SQLite and an identity
column on PostgreSQL, so a cursor for a pruned event cannot resolve to a newer
event. Verification is in
[capabilities](../../capabilities.md#item-event-short-id-evidence).

## User, security, compatibility, and operational risk

- Any database that already applied version 000004 keeps the text-keyed
  `item_events` table. The new build does not reapply it and fails on integer
  keys. Such databases cannot be upgraded in place.
- Change-feed cursors issued before this change are malformed. Clients that
  stored one receive `resync` on their next connection and must refresh the item
  list. Replay history before the reset is lost.
- The `item.changed` event `id` changes from an opaque hex string to a short ID.
- Authorization, audit semantics, and secret handling are unchanged.

## Compensating controls

- The operations guide states that this version must not run against an earlier
  migrated database ([operations](../../operations.md#upgrades-and-alerts)).
- A SQLite test proves that event keys are never reused after deletion, and the
  full suite runs on SQLite and on a fresh PostgreSQL container.

## Replacement or removal path

Before any release with deployed instances, a forward migration path is required
if in-place conversion is needed. Otherwise, remove this exception once the
release notes state the reset requirement. Review on or before 2027-02-09.

## Verification

- Focused tests for items, web, database, maintenance, and id packages; see
  [capabilities](../../capabilities.md#item-event-short-id-evidence).
