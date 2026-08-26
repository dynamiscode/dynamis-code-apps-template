# HTTP and REST API

The server exposes health checks, its OpenAPI 3.1 contract, local browser
session endpoints, workspace identity management, and a workspace-scoped item
resource. The canonical
machine contract is [`api/openapi.json`](../api/openapi.json).

## Endpoints

- `GET /health/live` reports process liveness without dependencies.
- `GET /health/ready` checks the database with a bounded timeout.
- `GET /api/openapi.json` serves the exact embedded contract.
- `POST /api/v1/auth/login` creates an HTTP-only browser session and returns
  its CSRF token. `POST /api/v1/auth/logout` requires that token.
- `/api/v1/workspaces/{workspaceId}/items` proves shared, workspace-scoped
  list, create, read, and update behavior.
API resources use `Authorization: Bearer <token>`. The token must belong to the
path workspace and include the required `resources:read` or `resources:write`
scope. Export requires `workspace:export`. Browser session rules are
documented in [authentication](authentication.md).

- `GET /api/v1/workspaces/{workspaceId}/export` returns the bounded versioned
  workspace export described in [data lifecycle](data-lifecycle.md).
- Members: `GET /members`, `PATCH /members/{userId}`, `DELETE /members/{userId}`,
  and `POST /ownership`.
- Invitations: `GET/POST /invitations`, `POST /invitations/{invitationId}/resend`,
  and `DELETE /invitations/{invitationId}`.
- Current-user tokens: `GET/POST /tokens`, `PATCH/DELETE /tokens/{tokenId}`.
- Current-user sessions: `GET /api/v1/sessions` and
  `DELETE /api/v1/sessions/{sessionId}`.

Identity and workspace routes use `Authorization: Bearer <token>`. The token
must belong to the path workspace and include the permission required by the
operation: `members:read`, `members:manage`, `ownership:transfer`,
`invitations:manage`, or `workspace:read`. Item and export requirements remain
`resources:*` and `workspace:export`. REST intentionally has no workspace
list/create endpoint; those operations are browser-only.

Invitation create/resend returns `invitationUrl` and delivery status, never a
standalone secret. Token create returns `secret` once only. Session responses
contain metadata only and never session or CSRF secrets.

## Contract rules

Errors use RFC 9457 `application/problem+json` with a stable `type`, `code`,
HTTP `status`, safe `detail`, request `instance`, and `requestId`. Internal
errors never expose implementation details.

Collections accept only documented filters and `created_at` or `-created_at`
sorts. Opaque cursors include the stable sort position; page size defaults to
50 and cannot exceed 100. Unsupported or repeated query parameters fail.

Item reads return strong version ETags. Updates require `If-Match`; missing,
malformed, or stale versions return `428` or `412`. Creates require an
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
