# Exception: forward-only migration 000016 rewritten in place for OAuth token short IDs

- Profile and requirement: architecture invariant that migrations are
  forward-only ([architecture](../../architecture.md#data-and-scaling)); applied
  migration 000016 is rewritten in place to change `oauth_tokens.id` and
  `oauth_tokens.parent_id` from `TEXT` to integer keys.
- Status: accepted
- Responsible owner: repository maintainers (issue #243, ticket #247)
- Approving maintainer: repository maintainer, approved in the spec for issue
  #243 ("convert entity tables to the integer-key plus short-ID pattern") and
  the implementation request for ticket #247
- Security-owner approval, when required: not required; no security control is
  weakened. Token secrets stay random and are looked up only by `secret_hash`.
  A row's integer key or short ID is never accepted as a bearer token.
- Created: 2026-11-12
- Review by: 2027-02-10
- Expires: 2027-02-10

## Reason and evidence

Ticket #247 moves `oauth_tokens` to an integer primary key with the `oauth_token`
short ID, and converts the self-reference `parent_id` to match. `grant_id` and
`client_id` were already integers. The change follows the reset approach of the
sharing, SCIM, and MCP OAuth exception
([2026-11-11-sharing-scim-oauth-short-id-migration-rewrite](2026-11-11-sharing-scim-oauth-short-id-migration-rewrite.md)),
which rewrote the same migration. A forward migration would leave a `TEXT` key
in the historical schema and add a conversion for a table whose rows are
short-lived credentials. Verification is in
[capabilities](../../capabilities.md#oauth-token-short-id-evidence).

## User, security, compatibility, and operational risk

- Any database that already applied migration 000016 keeps the old `TEXT` token
  keys and records the version as applied. The new build does not reapply it and
  fails on integer keys. Such databases cannot be upgraded in place and must be
  recreated, as the sharing, SCIM, and MCP OAuth exception already requires.
- Access, refresh, and code rows that exist when the database is recreated are
  lost. Active MCP grants must be re-consented after the reset.
- `oauth_tokens.parent_id` keeps no foreign key, as before. It is written only
  from the refresh path and is not read back by any API.
- Authorization and secret handling are unchanged. The principal's token ID, which
  appears in MCP request logs and audit actor fields, changes from the random row
  ID to the `oauth_token` short ID.

## Compensating controls

- The operations guide states that this version must not run against an earlier
  migrated database ([operations](../../operations.md#upgrades-and-alerts)).
- Tests assert that integer row IDs, decimal row IDs, and short row IDs are
  rejected as bearer tokens by access authentication, refresh, and revoke, and that
  the real access token still works after those probes.
- The migration runner renders the one dialect-specific fragment per driver and is
  covered by SQLite and PostgreSQL tests from a fresh database.

## Replacement or removal path

Before any release that has deployed instances, a forward migration path is
required if in-place conversion is needed. Otherwise, remove this exception once
the release notes state the reset requirement. Review on or before 2027-02-10.

## Verification

- Verification results are recorded in
  [capabilities](../../capabilities.md#oauth-token-short-id-evidence).
