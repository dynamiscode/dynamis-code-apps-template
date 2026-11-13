# 0011: Identity entities use integer keys with short-ID public forms

- Status: accepted
- Date: 2026-11-13
- Owners: template maintainers

## Context

Entity records were first keyed by random 128-bit hex text. Those text keys
are retired. Each entity table now stores an integer primary key, and the
only form that leaves the process is a short ID from the
[ADR 0009](0009-short-id-codec.md) codec. The change is tracked under the
short-ID migration exceptions recorded in `docs/decisions/exceptions/`.

Constraints:

- No entity ID is stored as text. Text stays only where the value is a
  credential or protocol state (session, CSRF, OAuth, MFA challenge,
  verification, reset, and recovery values), because those are secrets or
  opaque protocol tokens, not entity identifiers.
- Each public ID must be typed, so an ID for one kind of entity is not
  accepted as another kind.
- Migrations are forward-only. Converting a stored key is done by rewriting
  the applied migration in place under a recorded exception.

## Decision

Integer keys

- Entity tables use an integer primary key. `{{integer_pk}}` renders
  `INTEGER PRIMARY KEY` on SQLite and `BIGINT GENERATED ALWAYS AS IDENTITY` on
  PostgreSQL. SQLite file keys use `AUTOINCREMENT`, so a deleted key is never
  reused.
- Foreign references to entities are `BIGINT`, including user references and
  audit actors.
- Integer keys stay inside the process. Go types such as `identity.WorkspaceID`
  and `items.ItemID` marshal to their short IDs and accept only short IDs when
  parsed. An integer, a malformed value, or an unknown value gets the same
  not-found response as a missing record.

Short IDs

- Public entity IDs use the ADR 0009 codec. This record does not change the
  codec.
- Each entity kind has a type code from the registry in
  `internal/platform/id` (`TypeCodes()`). The registry holds 19 codes, and a
  test requires each code to be non-empty and unique. A type code seeds that
  kind's alphabet. It is never placed in the encoded string.
- The codec accepts an optional seed. Every codec in the repository is built
  with an empty seed, so each alphabet depends on the type code alone. No
  configuration value sets a seed. Changing a seed would re-encode every
  issued short ID, so a seed is not a runtime setting.

Strict decode

- `Codec.Decode` rejects a length outside 4 through the widest encoding of
  `MaxUint64`, a character outside the codec alphabet, and any input whose
  re-encoding differs from the input. `DecodeKey` also rejects zero and values
  above `MaxInt64`.
- Known limit: decoding does not reject a short ID issued under another type
  code. A small share of IDs from one type decode under another type's
  alphabet. Callers rely on the typed parser for that kind, and the lookup
  returns not-found when the record does not exist. See the webhook evidence
  in [capabilities](../capabilities.md#webhook-and-job-short-id-evidence).

Typed keys outside entity tables

- Audit target: `target_type` holds the registry type code and `target_key`
  holds a `BIGINT`. A target with no integer key stores `NULL` for the key.
- Audit actor: `actor_user_id` is a `BIGINT` user key. It is `NULL` for system
  events. Exports emit the user short ID.
- Idempotency principal: `principal_kind` (`TEXT`) and `principal_key`
  (`BIGINT`) replace the old principal string. The primary key is
  `(key_hash, principal_kind, principal_key, workspace_id, operation)`.
  Display uses `Principal.TokenShortID()`.
- Background job deduplication: `deduplication_key` is `BIGINT`. For webhook
  delivery jobs it is the delivery integer key.

Owner decisions

The owner approved both points below on 2026-11-13.

1. Principal kinds. The first design sketch named two kinds, `user` and
   `token`. The implementation also uses `scim_token` and `oauth_grant`,
   because those keys come from separate tables and a two-value kind would
   let integer keys collide. This also changes the OAuth replay scope: an MCP
   OAuth actor's idempotency scope moves from the access-token row, which
   changes on every refresh, to the grant. A retry after a token refresh now
   replays instead of creating a second item.
2. Key-less audit targets. `session`, `mfa_challenge`, and `external_identity`
   store `NULL` as the target key, because their identifiers are text
   credentials that stay text. Their audit rows no longer identify one object.
   `notification_preference` and `scim_group` lose only a category or role
   string, which moves to metadata. A surrogate integer key on each of those
   tables would restore object identity. That needs its own migration and
   decision.

## Consequences

Benefits

- Entity keys are compact integers in storage, and the public form is short,
  URL-safe, and typed.
- A malformed or foreign ID and a missing record return the same response.

Costs

- Applied migrations were rewritten in place for these keys. The migrations
  and the recorded exceptions are listed in
  [architecture](../architecture.md#ids-time-and-transactions). Upgrading resets
  existing data. The audit and idempotency exception is `proposed`.
- Audit target type names changed. `scim_user`, `api_token`, `public_share`,
  `passkey`, and `workspace_email` became `user`, `token`, `public_link`,
  `mfa_passkey`, and `workspace`. The MCP audit metadata field `token_id` is
  now `credential_id`.
- Passing no blocklist to the codec means a short ID could spell a banned
  word. That limit is marked in code and accepted in ADR 0009.
- The codec does not reject a short ID issued under another type code, so
  every route must use its own typed parser.

Rejected alternatives

- Random hex or UUID text keys for entities. Text keys need text columns in
  every reference and expose an unbounded string. They remain only for
  credentials.
- A runtime-configured seed. It would re-encode every issued ID when changed,
  and no requirement needs that.

## Revisit when

- A route accepts more than one type code, which would make the cross-type
  limit a correctness problem for that route.
- The owner rejects either decision above, or an audit or idempotency record
  needs object identity that a key-less target cannot provide.
- A word-blocking requirement applies to short IDs. Then enable the Sqids
  default blocklist as ADR 0009 describes.
- PostgreSQL becomes required for multiple instances, which changes how keys
  are allocated, not their form.
