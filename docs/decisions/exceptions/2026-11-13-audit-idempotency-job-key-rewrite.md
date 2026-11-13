# Exception: forward-only migrations rewritten for audit, idempotency, and job keys

- Profile and requirement: architecture invariant that migrations are
  forward-only ([architecture](../../architecture.md#data-and-scaling)) and the
  ADR 0009 rule that a stored entity ID is an integer key with a short-ID public
  form ([ADR 0009](../0009-short-id-codec.md)). Applied migrations 000002
  (`audit_events`), 000003 (`idempotency_records`), 000010 (`background_jobs`),
  and 000013 (`background_jobs` backfill from webhook deliveries) are rewritten in
  place. In `audit_events`, `actor_user_id` changes from `TEXT` to `BIGINT`, and
  `target_id TEXT` is replaced by `target_key BIGINT`. In `idempotency_records`,
  `principal_id TEXT` is replaced by `principal_kind TEXT` and `principal_key BIGINT`.
  In `background_jobs`, `deduplication_key` changes from `TEXT` to `BIGINT`.
- Status: accepted (approved by repository owner 2026-11-13)
- Responsible owner: repository maintainers (spec #243, ticket #195)
- Approving maintainer: repository owner, 2026-11-13. The owner decision on #243
  (comment https://github.com/dynamiscode/dynamis-code-apps-template/issues/243#issuecomment-6860178488)
  requires no narrowing of "no text entity ID column remains in the migrations",
  and this record is the in-place reset that follows from it. The owner accepted the
  reset; the two open design points (principal kinds and key-less audit targets) are
  not covered by this acceptance and remain open on #243.
- Security-owner approval, when required: approved by the security owner, 2026-11-13. Audit and idempotency records
  stop storing session IDs and MFA challenge tokens (see Known limitations), which
  narrows what the audit log holds. No authorization check changes. Sessions, OIDC
  state, MFA challenge, OAuth token, verification, reset, and recovery values stay
  text or hash keys and are never converted.
- Created: 2026-11-13
- Review by: 2027-02-11
- Expires: 2027-02-11

## Reason and evidence

Ticket #195 requires that no text entity ID remains in the migrations. The
owner's amendment excludes only credentials, secrets, hashes, and protocol
identifiers. Four columns still held entity IDs as text after #249 and #250:
`audit_events.actor_user_id` and `audit_events.target_id` (short IDs), and
`idempotency_records.principal_id` (`user:<short ID>` or `token:<short ID>`), and
`background_jobs.deduplication_key` (a webhook delivery's integer key as text).
Each is now typed. A forward migration would rebuild `audit_events`,
`idempotency_records`, and `background_jobs` in SQLite and PostgreSQL and map
every old text value. The users, workspace, and MFA exceptions chose the same
in-place reset for the same reason, so this change follows that approach.

Design, by column:

1. `audit_events.actor_user_id`: `BIGINT`, the user's integer key (#249). No
   foreign key, as before, so audit rows outlive a deleted user. `NULL` means a
   system event. The writer stores `UserID`. The history reader rejects a stored
   key that is not positive (`ErrInvalidShortID`). The export emits the user's
   short ID.
2. `audit_events.target_id` split into `target_type` (`TEXT`, the entity type code
   from `internal/platform/id`) and `target_key` (`BIGINT`, the entity's own integer
   key). Keys from different tables never collide because the type code is part of
   the target. The export derives `targetId` as `id.EncodeKey(target_type, target_key)`,
   and omits it when there is no key. `EncodeKey` returns `""` for an unregistered
   code instead of panicking.
3. `idempotency_records.principal_id` split into `principal_kind` (`TEXT`, one of
   `user`, `token`, `scim_token`, `oauth_grant`) and `principal_key` (`BIGINT`). The
   primary key is `(key_hash, principal_kind, principal_key, workspace_id, operation)`.
   A session principal is `user` with the user key. An API token is `token` with
   the `api_tokens` key. A SCIM token is `scim_token` with its key. An MCP OAuth
   actor is `oauth_grant` with the grant's key. `Principal.TokenID` is replaced by
   `TokenKind` and `TokenKey`. The only display string is
   `Principal.TokenShortID()`, derived from those columns, and it appears in MCP
   audit metadata. No `user:` or `token:` string is stored.
4. `background_jobs.deduplication_key`: `BIGINT`. It embeds a webhook delivery's
   integer key, so it is an entity key and is typed. `jobs.Queue.EnqueueTx` takes an
   `int64` key that must be positive. The job payload JSON is unchanged.

Deviation from the owner's sketch: `principal_kind` also holds `scim_token` and
`oauth_grant`, not only `user` and `token`. API tokens, SCIM tokens, and OAuth
grants are keyed in three different tables, so a two-value kind would let the
same integer key from two tables collide. Each kind is the registered type code
of its table. This needs owner approval.

Verification is in [capabilities](../../capabilities.md#audit-and-idempotency-short-id-evidence).

## User, security, compatibility, and operational risk

- Any database that already applied one of the rewritten versions keeps the text
  columns. The new build does not reapply those versions, so audit writes and
  idempotency lookups fail on the old schema. Such databases cannot be upgraded in
  place. This is a data-loss reset, not a conversion.
- No value mapping is provided. Existing audit rows, idempotency records, and
  queued deduplication keys are discarded by the reset.
- Target type names change for these records. `scim_user` becomes `user`,
  `api_token` becomes `token`, `public_share` becomes `public_link`, `passkey`
  becomes `mfa_passkey`, and `workspace_email` becomes `workspace` (the workspace
  email settings row is one per workspace). Exported `targetType` values change
  with them. The audit export is the only consumer of these names.
- OAuth idempotency scope changes. A key was scoped to the access token row, which
  changes on each refresh. It is now scoped to the grant, so a retry after a token
  refresh replays the original result instead of creating a second item.
- MCP audit metadata renames `token_id` to `credential_id`. The value is the short
  ID of the credential (grant, API token, or SCIM token) and is `""` for a session.
- The owner has not yet approved the reset. Until this record is accepted, this
  change must not be released to an instance that holds data.

## Known limitations

- Targets with no integer key store `target_key` as `NULL`: `session`,
  `mfa_challenge`, `external_identity`, `notification_preference`, and `scim_group`.
  Their identifiers are bearer or credential values (session, MFA challenge,
  external identity text key) or category names (notification type, SCIM role).
  Audit rows for these targets no longer carry that identifier. The notification
  type and SCIM role move to metadata where they were already recorded there. For
  the session, MFA challenge, and external identity events, an audit row can no
  longer be tied to one session, challenge, or external identity. Each row keeps
  its own audit event ID.
- Audit `target_type` values not in the type-code registry (`session`,
  `mfa_challenge`, `external_identity`, `notification_preference`, `scim_group`)
  are discriminators, not entity codes. They have no `targetId`.
- Audit rows, idempotency records, and queued deduplication keys written before
  this change are not converted, because the databases are reset.

## Compensating controls

- The migration SQL uses the shared `{{integer_pk}}` fragment and BIGINT for the
  integer keys. Fresh-database schema is checked on SQLite and isolated PostgreSQL
  by `TestSQLiteShortIDKeyColumns` and `TestPostgresShortIDKeyColumns`, which also
  assert that `audit_events.target_id` and `idempotency_records.principal_id` no
  longer exist.
- A stored audit actor that is not a positive key is rejected on read
  (`TestListAuditHistoryRejectsUnparseableActorRow`).
- Audit targets store only the typed key. `TestAuditTargetStoresTypedKeyOrNull`
  checks that a keyed target stores its type and key, and that a key-less target
  stores `NULL`. The export contract test
  (`TestSQLiteWorkspaceExport`, `TestPostgresWorkspaceExport`) checks that an
  exported item event carries the item's short ID and the actor's short user ID.
- Idempotency is scoped by the authenticating credential. An API-token principal
  does not share a scope with the user's session
  (`TestIdempotencyPrincipalIsTypedKey`).
- Job keys must be positive (`TestEnqueueRejectsNonPositiveDedupKey`).

## Replacement or removal path

Before any release with deployed instances, a forward migration path is required
if in-place conversion is needed. Otherwise, remove this exception once the
release notes state the reset requirement and the owner accepts the record.
Review on or before 2027-02-11.

## Verification

- Verification results are recorded in
  [capabilities](../../capabilities.md#audit-and-idempotency-short-id-evidence).
