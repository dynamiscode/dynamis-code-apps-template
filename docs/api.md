# HTTP and REST API

The server exposes health checks, workspace identity management, optional Clients, and private Files. The canonical machine contract is [`api/openapi.json`](../api/openapi.json); the live reference is `GET /api/openapi.json`.

Quick check:

```sh
curl http://127.0.0.1:8080/api/openapi.json
```

## Consumer quickstart

API tokens are created from the workspace Settings API-token screen or through
`POST /api/v1/workspaces/{workspaceId}/tokens` using an existing bearer token
with `workspace:read`. The token secret is returned once. Keep it in an
environment variable; do not commit or log it.

```sh
export BASE_URL=http://127.0.0.1:8080 # no trailing slash
export WORKSPACE_ID='<workspace short ID>'
export TOKEN='<token secret>'
```

Use `resources:read` for Clients reads and `resources:write` for Clients writes when the Clients profile is enabled. Tokens must belong to the workspace in the URL. See [Clients endpoints](#endpoints) for supported operations.

Errors use RFC 9457 Problem Details. Missing or invalid credentials return `401`; insufficient scope returns `403`; malformed, unknown, or foreign workspace IDs return `404`. Stale conditional writes return `412`.

Every problem includes `type`, `status`, `detail`, `instance`, `code`, and
`requestId`. Include `X-Request-ID` on requests when correlating a client log;
the server returns the accepted or generated request ID.

## Endpoints

- `GET /health/live` reports process liveness without dependencies.
- `GET /health/ready` checks the database with a bounded timeout.
- `GET /api/openapi.json` serves the exact embedded contract.
- `POST /api/v1/auth/login` creates an HTTP-only browser session and returns
  its CSRF token, or returns a short-lived MFA challenge when a configured
  factor is required. Complete that challenge with
  `POST /api/v1/auth/mfa/passkey`, `/mfa/totp`, or `/mfa/recovery`.
  `POST /api/v1/auth/logout` requires the session CSRF token.
- MFA status and factor management use the session-cookie endpoints under
  `/api/v1/auth/mfa`. Enrollment/removal requires CSRF plus fresh
  authentication; a current password is accepted, while a recent OIDC sign-in
  or MFA-authenticated session can satisfy the same step-up. Enrollment
  responses show recovery codes once only.
  TOTP enrollment material is `no-store`; factor secrets are never returned
  after enrollment. Passkey IDs in list and removal (`DELETE
  /api/v1/auth/mfa/passkeys/{passkeyId}`) are short IDs; a malformed or unknown
  ID returns the same `mfa-invalid` problem as a missing passkey. Challenge tokens and
  recovery codes are secrets, never row IDs.
API resources use `Authorization: Bearer <token>`. The token must belong to the
path workspace and include the required `resources:read` or `resources:write`
scope. Export requires `workspace:export`. Browser session rules are
documented in [authentication](authentication.md).

- `GET /api/v1/workspaces/{workspaceId}/export` returns the bounded versioned
  workspace export described in [data lifecycle](data-lifecycle.md).
- Members: `GET /members` returns all members unless `limit` or `page` is
  supplied; with either parameter it returns `total`, the number of members,
  and the `page` returned. Also `PATCH /members/{memberId}`, `DELETE /members/{memberId}`,
  and `POST /ownership` with body `{"memberId": "<short ID>"}`.
- Invitations: `GET/POST /invitations`, `POST /invitations/{invitationId}/resend`,
  and `DELETE /invitations/{invitationId}`.
- Current-user tokens: `GET/POST /tokens`, `PATCH/DELETE /tokens/{tokenId}`.
- Member, invitation, and token paths take short IDs only. A malformed,
  unknown, or foreign ID returns `404`, code `not-found`, with the same body.
  Token secrets are random values, not encoded IDs, and are returned once on create.
- Current-user sessions: `GET /api/v1/sessions` and
  `DELETE /api/v1/sessions/{sessionId}`.
- Webhooks: `GET/POST /api/v1/workspaces/{workspaceId}/webhooks`,
  `DELETE/POST /api/v1/workspaces/{workspaceId}/webhooks/{webhookId}` (delete
  or rotate at `/secret`), and `GET /api/v1/workspaces/{workspaceId}/webhooks/{webhookId}/deliveries`.
- Files: `GET/POST /api/v1/workspaces/{workspaceId}/files` lists, uploads, or
  initiates a file; `GET /files/{fileId}` returns metadata and a short-lived
  download URL; `PUT /files/{fileId}/content` streams the bytes of an initiated
  upload through the application on every driver, and the file becomes ready
  there for local storage. On S3 the file stays pending until
  `POST /files/{fileId}/complete` verifies the stored object, and the initiate
  response's `uploadUrl` points at the content route. File IDs are the file
  short ID (ADR 0009, type `file`) and the only form in paths and bodies;
  malformed, unknown, and integer-key IDs return the same not-found problem.
- SCIM: create/revoke the dedicated credential with
  `POST/DELETE /api/v1/workspaces/{workspaceId}/scim-token`, then use
  `GET/POST /scim/v2/{workspaceId}/Users`, `GET/PATCH/DELETE
  /scim/v2/{workspaceId}/Users/{userId}`, `GET
  /scim/v2/{workspaceId}/Groups`, and `GET/PATCH
  /scim/v2/{workspaceId}/Groups/{admin|member|viewer}`. SCIM uses
  `application/scim+json`, exact `userName`/`externalId` filters, bounded
  `startIndex`/`count` pagination, strong ETag/If-Match, and safe SCIM error
  responses. Group ETags advance for SCIM user changes and ordinary workspace
  membership changes, including both sides of a role move. Group PATCH supports
  member arrays and bounded filtered member removals. User DELETE deactivates
  the workspace mapping; POST retries without `externalId` return the existing
  normalized workspace mapping.
- SCIM resource `id` and group `members.value` are the user's short ID. The
  `{userId}` path takes only that short ID; the IdP `externalId` is stored and
  filtered as an IdP value but is never accepted in a path or member value.
  Malformed, unknown, and foreign IDs, and external IDs, return the same SCIM
  not-found error. When `externalId` is omitted, the stored external ID defaults
  to the user's short ID.
- User IDs in responses and exports are short IDs only: `userId` on tokens and
  sessions, `ExportMember.userId`, file `ownerUserId`,
  and audit `actorUserId` and user `targetId`. Integer keys are never emitted.
  An audit `targetId` is the short ID of the entity named by `targetType`; a
  target with no integer key (a session, MFA challenge, external identity,
  notification preference, or SCIM group) has no `targetId`.
- Audit `targetType` for a keyed target is that entity's registry type code,
  for example `user`, `token`, `public_link`, `mfa_passkey`, or `workspace`
  (registry in `internal/platform/id`). Key-less targets use their own names (`session`, `mfa_challenge`, `external_identity`,
  `notification_preference`, `scim_group`). The earlier names `scim_user`,
  `api_token`, `public_share`, `passkey`, and `workspace_email` are retired.

Identity and workspace routes use `Authorization: Bearer <token>`. The token
must belong to the path workspace and include the permission required by the
operation: `members:read`, `members:manage`, `ownership:transfer`,
`invitations:manage`, or `workspace:read`. Client and export requirements are `resources:*` and `workspace:export`. REST intentionally has no workspace
list/create endpoint; those operations are browser-only.

Token create returns `secret` once only; token lists never return it. Invitation
create/resend returns an invitation URL and delivery status, not a standalone
secret. Session responses contain metadata only and never session or CSRF
secrets. Problem details and logs exclude credentials, authorization headers,
session values, invitation values, connection strings, and signed URLs.

- Engagement stages: `GET` and `POST /api/v1/workspaces/{workspaceId}/engagement-stages` list and append stages. Reads require `resources:read`; mutations require `workspace:update` and an owner or admin role. Supply a nonblank name of at most 200 characters and kind `open`, `won`, or `lost`. New workspaces have no stages. Reorder with `POST .../engagement-stages/reorder` and an `ids` array containing every stage short ID exactly once; archive with `POST .../engagement-stages/{stageId}/archive`. Lists include archived stages and their archive state. Reordering does not change kind. Both mutations are audited.
- Engagements: `POST /api/v1/workspaces/{workspaceId}/engagements` requires `resources:write`, a workspace client short ID, optional open stage short ID, and optional notes. Without a stage ID, the first unarchived open stage is selected. Creation fails if no open stage exists or the selected stage is closed or archived. `GET .../engagements/{engagementId}` requires `resources:read`. Creation is audited. A client with engagements cannot be deleted.
- Next actions: `PUT .../engagements/{engagementId}/next-action` sets text, an optional `dueDate` (YYYY-MM-DD), and `ownerId` (a workspace member's user short ID); an empty object clears it. Changes require `resources:write` and are audited. `GET .../engagements/due` requires `resources:read` and lists open-stage engagements due on or before the current UTC date. Stage moves preserve the action.
- Quotes: `PUT .../engagements/{engagementId}/quote` sets `status` to `not_sent`, `sent`, `pending`, `accepted`, or `declined`, with optional `amount` (non-negative decimal string, up to 12 integer and two fractional digits) and `dateSent` (YYYY-MM-DD). New engagements default to `not_sent`. Changes require `resources:write` and are audited; stage moves preserve the quote. No document or signature is created.
- Assignment: `PUT .../engagements/{engagementId}/assignee` sets `assigneeId` to a workspace member's user short ID; an empty value or object clears it. Changes require `resources:write` and are audited. Assignment survives stage changes.
- Agreement: `PUT .../engagements/{engagementId}/agreement` records or clears `recorded` and an optional `fileId`. The file short ID must identify a ready file in the same workspace. Changes require `resources:write` and are audited. The reference grants no file access and survives stage changes.
- Clients (Clients profile): `GET` and `POST /api/v1/workspaces/{workspaceId}/clients`
  list and create clients. `GET` and `PATCH /api/v1/workspaces/{workspaceId}/clients/{clientId}`
  read and edit one client; `PATCH` requires `If-Match` with the client ETag.
  `POST .../clients/{clientId}/archive` sets `archivedAt` and `POST .../unarchive`
  clears it. Both are idempotent, need write scope (members and above), and return
  the client with its ETag. `DELETE .../clients/{clientId}` permanently deletes a
  client and its custom values in one transaction. It needs owner or admin
  rights and returns `204`. Viewers and members get `forbidden` (403).
  The list pages newest first by default, numbered from `page`, takes the `sort` and
  `filter` parameters below, and has no free-text search in v1.
  Client IDs are short IDs. Malformed, integer, unknown, and foreign IDs return
  the same `not-found` problem. Viewers read; members and above write.
- Client timeline: `GET .../clients/{clientId}/timeline` returns manual entries in ascending date order, 50 per page by default; `limit` accepts 1–100 and `page` is 1-based (maximum 1,000,000). Continue until a page contains fewer entries than the limit. `POST` appends an entry with `type`, `text`, optional `channel`, and optional `correctsId` (an entry of the same client). Types are `note`, `call`, `visit`, `document`, `update`, `change`, and `delay`; channels are `phone`, `whatsapp`, `email`, `in_person`, and `other`. Text is required and limited to 2,000 characters after trimming. Entries include author, creation time, and correction reference; they cannot be edited or deleted. Reads require client read permission; writes require client write permission.
- Client custom values: `GET .../clients/{clientId}/values` lists the active
  fields with each value (`null` when unset). `PUT .../clients/{clientId}/values/{fieldId}`
  stores a text value (`{"value": "..."}`, 1 to 500 characters after trimming) and
  `DELETE` on the same path clears it. Both need `If-Match` with the client ETag and
  return the client with a new ETag. Archived fields refuse writes. A blank value
  is refused; use `DELETE` to clear. A value is returned as text: a canonical
  decimal for `number`, `YYYY-MM-DD` for `date`, and `true` or `false` for
  `yes_no`.
- Client list query: `sort=<field>` orders the list by one field, and
  `sort=-<field>` descends. A field is `displayName`, `email`, `phone`, `notes`,
  `createdAt`, `updatedAt`, or a custom field's short ID. Number fields sort
  numerically, and other text sorts by bytes. Empty values sort last in both
  directions. Ties break by client ID in the same direction.
  `filter=<field>:<operator>:<value>` may repeat, and conditions combine with AND.
  The value may contain colons. `is_empty` takes an empty value (for example,
  `filter=email:is_empty:`) and matches absent, null, or empty values. Operators by field kind:
  - Text (`displayName`, `email`, `phone`, `notes`, and custom text): `equals` or
    `contains`. Matching is case-insensitive for ASCII letters only, in both
    dialects. Non-ASCII letters match as typed.
  - Number, date, and timestamps (`createdAt`, `updatedAt`): `equals`, `before`,
    `after`, or `between`. Timestamps take RFC3339 values. `before` and `after`
    are strict. `between` takes `from,to` and is inclusive. A number's `equals`
    compares canonical decimal text, so `10.0` matches `10`.
  - Yes/no: `equals` with `true` or `false`.
  - Single choice: `equals` with one or more comma-separated option IDs. It
    matches any listed option.
  `archived=<value>` selects which clients the list shows. `active` (the default)
  excludes archived clients, `archived` shows only archived clients, and `all`
  shows both. Any other value returns `invalid-request` (400). A sort or filter
  that is unknown, archived, or malformed returns `invalid-request` (400).
  A client list always includes `total`, the number of clients the filters and archived value match across all pages. Clients have no free-text search in v1.
- Client custom fields: `GET` and `POST /api/v1/workspaces/{workspaceId}/client-fields`
  list fields (archived ones only with `includeArchived=true`) and define a text,
  number, date, or yes/no field (`type`: `text`, `number`, `date`, or `yes_no`).
  `POST .../client-fields/{fieldId}/archive` and `.../restore` change a
  field's archive state. These writes need the owner or admin role. A field has
  no slot in responses. Refused definitions return `resource-limit` (409) when
  `CLIENT_FIELDS_MAX_PER_WORKSPACE` is reached.

## Webhooks

Webhook management requires `webhooks:read` or `webhooks:manage`. Creation and
secret rotation return a secret once; all later responses omit it. Delivery
requests include `Webhook-Id`, `Webhook-Timestamp`, and
`Webhook-Signature: v1,<base64>` where the HMAC input is
`Webhook-Id + "." + Webhook-Timestamp + "." + body`. Consumers must verify the
signature, reject stale timestamps, and deduplicate by `Webhook-Id` because
retries are at-least-once. The server records at most five attempts and
exposes only redacted delivery status and error categories.

Webhook, delivery, and event IDs are short IDs, and `Webhook-Id` is the event's
short ID (type `webhook_event`). Integer keys and malformed IDs are never
accepted: a malformed, integer, or unknown webhook ID gets the same not-found
problem as a missing webhook.

## Contract rules

Errors use RFC 9457 `application/problem+json` with a stable `type`, `code`,
HTTP `status`, safe `detail`, request `instance`, and `requestId`. Internal
errors never expose implementation details.

Common error codes are `invalid-request`, `not-found`, `precondition-required`,
`precondition-failed`, `idempotency-key-required`, `idempotency-conflict`,
`insufficient-scope`, and `rate-limited`. Clients should branch on `status` and
stable `code`, not on human-readable `title` or `detail`.

Clients collections support numbered pages, documented filters and sorting. Client updates require `If-Match` with the current ETag; stale writes return `412`. See the Clients endpoint contract above.

## Compatibility

Additive response fields and new endpoints are backward compatible. Clients
must ignore unknown response fields. Removing or changing a field, status,
operation, or meaning requires a new major path such as `/api/v2`.

Deprecated operations remain available for at least one published minor
release and 90 days. Mark them in OpenAPI with `deprecated: true`, publish the
replacement and removal date, and keep generated artifacts synchronized with:

```sh
go generate ./api
git diff --exit-code -- api/contract.gen.go
```
