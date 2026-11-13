# Exception: forward-only migrations rewritten for users short IDs

- Profile and requirement: architecture invariant that migrations are
  forward-only ([architecture](../../architecture.md#data-and-scaling)). Applied
  migrations 000002 (users, external identities, instance admins, workspace
  members, sessions, invitations, API tokens), 000003 (items
  `created_by_user_id`), 000005 (OIDC transaction user),
  000007 (account, notification, and preference tables), 000012 and 000013
  (SCIM tokens and users), 000014 (files, superseded by 000020), 000015 (MFA),
  000016 (OAuth grants), and 000020 (files rebuild) are rewritten in place.
  Every `user_id`, `owner_user_id`, `created_by_user_id`, and
  `invited_by_user_id` column that references `users(id)` changes from `TEXT`
  to `BIGINT`. The `users` key becomes an integer key. `oidc_transactions.state_hash`
  stays `TEXT`: it is a hash of a bearer value, not an entity ID, and it is not
  converted by this change.
- Status: proposed (owner approval pending)
- Responsible owner: repository maintainers (spec #243, ticket #249)
- Approving maintainer: pending. The owner decisions on #243 (comment
  https://github.com/dynamiscode/dynamis-code-apps-template/issues/243#issuecomment-6860178488)
  choose the in-place reset and the `{{file_id_pk}}` key for users, but this record
  is not yet approved.
- Security-owner approval, when required: pending. No security control is
  weakened. Sessions, OIDC state, MFA challenge, recovery-code, and token
  secrets stay text keys matched by hash. `external_identities.id` stays text
  and is never accepted as a user key.
- Created: 2026-11-13
- Review by: 2027-02-11
- Expires: 2027-02-11

## Reason and evidence

Ticket #249 requires `users` and its 21 referencing columns to use integer keys
and the `user` type code, so a user's short ID is the only public form of that
key. A forward migration would have to rebuild `users` and about twenty child
tables in SQLite, because SQLite cannot change a column or primary key type in
place. It would also have to rebuild or alter each child foreign key in
PostgreSQL and map every old text identifier to a new integer key. The workspace,
MFA, and notification exceptions chose the same reset for the same reason, so
this change follows that approach. A `{{file_id_pk}}` key (AUTOINCREMENT on
SQLite) keeps a deleted user's short ID from being reused. Verification is in
[capabilities](../../capabilities.md#users-short-id-evidence).

## User, security, compatibility, and operational risk

- Any database that already applied one of the rewritten versions keeps the
  text-keyed `users` table and child columns. The new build does not reapply
  those versions, so login, invitation, token, and MFA paths fail on the old
  schema. Such databases cannot be upgraded in place. Existing user accounts,
  sessions, memberships, API tokens, and MFA enrollments are lost on upgrade.
  This is a data-loss reset, not a conversion.
- No value mapping from old text user IDs to integer keys is provided.
- The owner has not yet approved the reset. Until this record is accepted, the
  change must not be released to an instance that holds accounts.
- Authorization, audit semantics, signing, and secret handling are unchanged.
  Audit `actor_user_id` and `target_id` now store short IDs; REST, MCP, and web
  user identifiers emit short IDs. The MCP `createdByUserId` field keeps its
  string schema. Those output names and contract surfaces are the scope of #250.
- Codec collision risk ([ADR 0009](../0009-short-id-codec.md)): the codec does
  not bind a short ID to its type code. A user short ID can therefore decode
  under another type code, and the same integer can produce the same short ID
  under two type codes. Callers must decode a user short ID only with the user
  codec, which the strict `ParseUserID` path does. The collision is accepted here
  as the webhook and workspace records accepted it.

## Known limitations

- `CreateSCIMUser` now defaults `external_id` to the user short ID when the
  request omits it (`internal/identity/scim.go`, `externalID = userID.String()`).
  Previously the default was the raw integer key.
- The SCIM backfill in migrations 000012 and 000013 casts the user key to decimal
  text for `scim_users.external_id`. It runs only over existing
  `workspace_members`, so it is inert on a reset database.
- Audit `actor_user_id` and `target_id`, and `idempotency_records.principal_id`
  (`user:<short ID>`), now hold short IDs. Values written before this change are
  not converted, because the databases are reset.

## Compensating controls

- The operations guide states that this version must not run against an earlier
  migrated database ([operations](../../operations.md#upgrades-and-alerts)).
- The migration SQL uses the shared `{{integer_pk}}` and `{{file_id_pk}}`
  fragments, rendered per driver. Fresh-database schema is covered by SQLite and
  isolated PostgreSQL tests, including a check that the 21 referencing columns
  and `users.id` are `BIGINT` on PostgreSQL.
- Malformed, zero, and integer user IDs are rejected by `ParseUserID` and the
  JSON decoder (`TestParseUserIDRejectsMalformedAndZero`,
  `TestUserIDJSONEmitsShortIDNotIntegerKey`).

## Replacement or removal path

Before any release with deployed instances, a forward migration path is required
if in-place conversion is needed. Otherwise, remove this exception once the
release notes state the reset requirement and the owner accepts the record.
Review on or before 2027-02-11.

## Verification

- Verification results are recorded in
  [capabilities](../../capabilities.md#users-short-id-evidence).
