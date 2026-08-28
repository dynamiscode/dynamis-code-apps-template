# HTTP and REST API

The server exposes health checks, its OpenAPI 3.1 contract, local browser
session endpoints, workspace identity management, a workspace-scoped item
resource, and (when the Files profile is selected) private workspace files. The canonical machine contract is [`api/openapi.json`](../api/openapi.json);
the live reference is `GET /api/openapi.json`.

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
export WORKSPACE_ID='0123456789abcdef0123456789abcdef'
export TOKEN='<token secret>'
```

Use `resources:read` for item list/read and `resources:write` for item
create/update/delete. A token must belong to the workspace in the URL.

Check liveness and discover the machine contract:

```sh
curl --silent --show-error "$BASE_URL/health/live"
curl --silent --show-error "$BASE_URL/api/openapi.json"
```

List items. `nextCursor` is present only when another page exists:

```sh
curl --silent --show-error --include \
  -H "Authorization: Bearer $TOKEN" \
  "$BASE_URL/api/v1/workspaces/$WORKSPACE_ID/items?limit=2&sort=-created_at"

# Copy nextCursor exactly; keep every other query parameter unchanged.
export NEXT_CURSOR='<nextCursor from the previous response>'
curl --silent --show-error --include --get \
  -H "Authorization: Bearer $TOKEN" \
  --data-urlencode 'limit=2' \
  --data-urlencode 'sort=-created_at' \
  --data-urlencode "cursor=$NEXT_CURSOR" \
  "$BASE_URL/api/v1/workspaces/$WORKSPACE_ID/items"
```

Create an item. The key is required, scoped to the token, workspace, and
operation, and retained for 24 hours. Retrying the same key with the same body
replays the original response; changing the body returns `409` with code
`idempotency-conflict`.

```sh
export IDEMPOTENCY_KEY='onboarding-create-1'
curl --silent --show-error --include \
  -X POST "$BASE_URL/api/v1/workspaces/$WORKSPACE_ID/items" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $IDEMPOTENCY_KEY" \
  --data '{"title":"Onboarding item"}'
```

Copy the returned `id` and `ETag` (for example, `"v1"`) into `ITEM_ID` and
`ETAG`. Reads return the current strong ETag; updates and deletes require that
value in `If-Match` and return a new ETag after an update.

```sh
export ITEM_ID='0123456789abcdef0123456789abcdef'
export ETAG='"v1"'

curl --silent --show-error --include \
  -H "Authorization: Bearer $TOKEN" \
  "$BASE_URL/api/v1/workspaces/$WORKSPACE_ID/items/$ITEM_ID"

# Returns 304 with no body when the item has not changed.
curl --silent --show-error --include \
  -H "Authorization: Bearer $TOKEN" \
  -H "If-None-Match: $ETAG" \
  "$BASE_URL/api/v1/workspaces/$WORKSPACE_ID/items/$ITEM_ID"

curl --silent --show-error --include \
  -X PATCH "$BASE_URL/api/v1/workspaces/$WORKSPACE_ID/items/$ITEM_ID" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -H "If-Match: $ETAG" \
  --data '{"status":"complete"}'

# Use the update response's new ETag.
export ETAG='"v2"'
curl --silent --show-error --include \
  -X DELETE "$BASE_URL/api/v1/workspaces/$WORKSPACE_ID/items/$ITEM_ID" \
  -H "Authorization: Bearer $TOKEN" \
  -H "If-Match: $ETAG"
```

Errors use RFC 9457 Problem Details with content type
`application/problem+json`. Branch on `status` and stable `code`; treat
`title` and `detail` as display text. Common onboarding cases:

- Missing or invalid bearer token: `401`, code `unauthorized`, and
  `WWW-Authenticate: Bearer`.
- Token missing the required item scope: `403`, code `insufficient-scope`.
- Missing create key: `400`, code `idempotency-key-required`.
- Missing `If-Match`: `428`, code `precondition-required`; stale `If-Match`:
  `412`, code `precondition-failed`.
- Invalid parameters or cursor: `400`, code `invalid-request`; missing item:
  `404`, code `not-found`.
- Rate limit: `429`, code `rate-limited`; wait the number of seconds in
  `Retry-After` before retrying.

Every problem includes `type`, `status`, `detail`, `instance`, `code`, and
`requestId`. Include `X-Request-ID` on requests when correlating a client log;
the server returns the accepted or generated request ID.

## Endpoints

- `GET /health/live` reports process liveness without dependencies.
- `GET /health/ready` checks the database with a bounded timeout.
- `GET /api/openapi.json` serves the exact embedded contract.
- `POST /api/v1/auth/login` creates an HTTP-only browser session and returns
  its CSRF token. `POST /api/v1/auth/logout` requires that token.
- `/api/v1/workspaces/{workspaceId}/items` proves shared, workspace-scoped
  list, create, read, and update behavior.
  Item responses retain `createdByUserId` as a required field but return `null`
  after its creator's account is deleted; item content remains available.
API resources use `Authorization: Bearer <token>`. The token must belong to the
path workspace and include the required `resources:read` or `resources:write`
scope. Export requires `workspace:export`. Browser session rules are
documented in [authentication](authentication.md).

- `GET /api/v1/workspaces/{workspaceId}/export` returns the bounded versioned
  workspace export described in [data lifecycle](data-lifecycle.md).
- `POST /api/v1/workspaces/{workspaceId}/import` atomically imports bounded
  item records from that JSON export format or strict `title,status` CSV.
  It requires `workspace:update`; source IDs, timestamps, memberships, audit
  events, and credentials are ignored.
- Members: `GET /members`, `PATCH /members/{userId}`, `DELETE /members/{userId}`,
  and `POST /ownership`.
- Invitations: `GET/POST /invitations`, `POST /invitations/{invitationId}/resend`,
  and `DELETE /invitations/{invitationId}`.
- Current-user tokens: `GET/POST /tokens`, `PATCH/DELETE /tokens/{tokenId}`.
- Current-user sessions: `GET /api/v1/sessions` and
  `DELETE /api/v1/sessions/{sessionId}`.
- Webhooks: `GET/POST /api/v1/workspaces/{workspaceId}/webhooks`,
  `DELETE/POST /api/v1/workspaces/{workspaceId}/webhooks/{webhookId}` (delete
  or rotate at `/secret`), and `GET /api/v1/workspaces/{workspaceId}/webhooks/{webhookId}/deliveries`.
- Files: `GET/POST /api/v1/workspaces/{workspaceId}/files` lists, uploads, or
  initiates a file; `GET /files/{fileId}` returns metadata and a short-lived
  download URL; `PUT /files/{fileId}/content` finalizes app-streamed uploads;
  `POST /files/{fileId}/complete` verifies S3 uploads.

Identity and workspace routes use `Authorization: Bearer <token>`. The token
must belong to the path workspace and include the permission required by the
operation: `members:read`, `members:manage`, `ownership:transfer`,
`invitations:manage`, or `workspace:read`. Item and export requirements remain
`resources:*` and `workspace:export`. REST intentionally has no workspace
list/create endpoint; those operations are browser-only.

Token create returns `secret` once only; token lists never return it. Invitation
create/resend returns an invitation URL and delivery status, not a standalone
secret. Session responses contain metadata only and never session or CSRF
secrets. Problem details and logs exclude credentials, authorization headers,
session values, invitation values, connection strings, and signed URLs.

## Webhooks

Webhook management requires `webhooks:read` or `webhooks:manage`. Creation and
secret rotation return a secret once; all later responses omit it. Delivery
requests include `Webhook-Id`, `Webhook-Timestamp`, and
`Webhook-Signature: v1,<base64>` where the HMAC input is
`Webhook-Id + "." + Webhook-Timestamp + "." + body`. Consumers must verify the
signature, reject stale timestamps, and deduplicate by `Webhook-Id` because
retries are at-least-once. The server records at most five attempts and
exposes only redacted delivery status and error categories.

## Contract rules

Errors use RFC 9457 `application/problem+json` with a stable `type`, `code`,
HTTP `status`, safe `detail`, request `instance`, and `requestId`. Internal
errors never expose implementation details.

Common error codes are `invalid-request`, `not-found`, `precondition-required`,
`precondition-failed`, `idempotency-key-required`, `idempotency-conflict`,
`insufficient-scope`, and `rate-limited`. Clients should branch on `status` and
stable `code`, not on human-readable `title` or `detail`.

Collections use one query shape: optional resource filters, `search`, `sort`,
`limit`, and opaque `cursor`. The item reference accepts `status=active|complete`
and `search`, which is a trimmed, case-insensitive literal substring match on
`title`; `%`, `_`, and `\\` have no wildcard meaning. Search is limited to 100
Unicode characters. `sort` is `created_at` or `-created_at`; ties use `id` in
the same direction. `limit` defaults to 50 and is bounded to 1-100. A response
contains `nextCursor` only when another page exists. Cursors bind the complete
filter/search/sort query and must be sent unchanged. Unsupported, repeated,
empty, or malformed parameters fail with Problem Details `400`.

Item reads return strong version ETags in the form `"vN"`. `If-None-Match` can
return `304`. Updates and deletes require `If-Match`; missing, malformed, or
stale versions return `428` or `412`. Creates require an
`Idempotency-Key`. Keys are hashed and scoped to principal, workspace, and
operation for 24 hours. An identical retry replays the result; a changed
request returns `409`.

Item deletion also requires `If-Match` and permanently removes live data.
Backup copies remain subject to the retention policy documented for the
deployment.

All routes have bounded bodies and execution time. Authentication routes use
a stricter per-source rate limit. `429` responses include `Retry-After`.

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
