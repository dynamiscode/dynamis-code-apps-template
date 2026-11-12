# Exception: forward-only migrations rewritten for sharing, SCIM, and MCP OAuth short IDs

- Profile and requirement: architecture invariant that migrations are
  forward-only ([architecture](../../architecture.md#data-and-scaling)); applied
  migrations 000011, 000012, 000013, and 000016 are rewritten in place, and
  000002 changes only its dialect placeholder name.
- Status: accepted
- Responsible owner: repository maintainers (issue #194)
- Approving maintainer: repository maintainer, approved in the implementation
  request for issue #194 ("implement as written"), which requires integer record
  keys for these tables
- Security-owner approval, when required: not required; no security control is
  weakened. Public-share tokens, SCIM secrets, and OAuth codes, access tokens,
  and refresh tokens stay random and hashed as before.
- Created: 2026-11-11
- Review by: 2027-02-09
- Expires: 2027-02-09

## Reason and evidence

Issue #194 requires public share links, SCIM tokens, OAuth clients, and OAuth
grants to use integer primary keys (`INTEGER PRIMARY KEY` on SQLite,
`BIGINT GENERATED ALWAYS AS IDENTITY` on PostgreSQL) and to expose only their
ADR 0009 short IDs. The OAuth foreign keys `oauth_grants.client_id`,
`oauth_tokens.grant_id`, and `oauth_tokens.client_id` change to `BIGINT` to
match. A forward migration would leave `TEXT` keys in the historical schema and
add conversions across four tables. The change follows the reset approach of the
workspace exception ([2026-11-11-workspace-short-id-migration-rewrite](2026-11-11-workspace-short-id-migration-rewrite.md)).
Verification is in [capabilities](../../capabilities.md#sharing-scim-and-mcp-oauth-short-id-evidence).

## User, security, compatibility, and operational risk

- Any database that already applied versions 000011, 000012, 000013, or 000016
  keeps the old `TEXT` keys and records those versions as applied. The new build
  does not reapply them and fails on integer keys. Such databases cannot be
  upgraded in place. Items, files, webhooks, and members keep their existing key
  types until their own issues change them.
- `oauth_tokens.id` stays random because token rows are codes and tokens, which
  the issue keeps unencoded. `oauth_clients.client_id` stays random because it is
  the public OAuth client identifier.
- Deployed instances, if any exist, must be recreated or restored into a new
  database. No automatic conversion is provided.
- Authorization, audit semantics (audit targets now carry the short ID), and
  secret handling are otherwise unchanged. Malformed and foreign-type public
  link IDs return the same not-found as a missing link.

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

- Verification results are recorded in
  [capabilities](../../capabilities.md#sharing-scim-and-mcp-oauth-short-id-evidence).
