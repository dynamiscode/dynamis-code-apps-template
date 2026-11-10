# MCP (Agent profile)

This surface is included only when the application is generated with the
Agent profile. Core and Identity applications do not expose `/mcp`.

The authenticated Streamable HTTP endpoint is `POST /mcp`. It uses the
official Go SDK, the stateless MCP `2026-07-28` flow, and bounded legacy
`2025-11-25` initialization compatibility provided by that SDK.

Use the same bearer API tokens as REST. Discovery and read tools require
`resources:read`; mutation tools require `resources:write`. Every tool calls
the workspace-scoped item application service. No tool accesses SQL, shell,
filesystem, code execution, or external networks.

## Tools

| Tool | Scope | Safety |
|---|---|---|
| `items_list_v1` | `resources:read` | read-only, idempotent, closed-world; bounded search, limit 1-100, and opaque cursor |
| `items_get_v1` | `resources:read` | read-only, idempotent, closed-world |
| `items_create_v1` | `resources:write` | additive, idempotent with caller key, closed-world |
| `items_update_v1` | `resources:write` | destructive, idempotent with version, closed-world, approval required |
| `items_delete_v1` | `resources:write` | destructive, idempotent with version, closed-world, approval required |

Names include their contract version. A breaking schema, scope, output, or
side-effect change requires a new name such as `_v2`. Item outputs retain their
record after account deletion and return `createdByUserId: null` when the
creator no longer exists.

## Prompts

`prompts/list` and `prompts/get` expose three read-only prompt templates. All
require `resources:read`, use only shared item use cases, and return one user
message. The first two are listed here; `item_review_v1` is described under
[Prompts and completion](#prompts-and-completion). They never create, update, or delete data.

| Prompt | Arguments | Result |
|---|---|---|
| `items_summarize_v1` | `workspaceId` required; `status` optional, `active` or `complete` | Summarizes the newest 50 matching items |
| `items_triage_v1` | `workspaceId` required | Ranks the newest 50 active items and proposes next actions; recommends only |

- `workspaceId` is 32 lowercase hexadecimal characters on every prompt.
- Undeclared arguments, missing required arguments, malformed IDs, and
  unsupported status values return JSON-RPC `-32602` with the safe message
  `parameters are invalid`. Unknown prompt names use the SDK's standard error.
- Workspace authorization is enforced by the item use case. Foreign or
  unauthorized workspaces return `access is denied`.
- Item titles are untrusted data. They are JSON-encoded inside a
  `<workspace_items>` block, so encoding escapes markup and a title cannot close
  the block. The instruction tells the model to treat the block as data.
- A truncated result says that only the newest 50 matching items are included.
- The endpoint stays stateless. Prompt lists are static, so no list-changed
  notifications are sent and no `Mcp-Session-Id` is issued.

Tools use the SDK's typed `mcp.AddTool`: the Go input and output types derive
the published schemas, and the SDK validates arguments before the handler runs
and validates outputs before they are returned. `testdata/tools-v1.json` pins
the published `tools/list` contract, and a contract test fails on any drift.
Arguments the schema rejects return `parameters are invalid` and write one
failed audit record without workspace or item identifiers, because the caller's
workspace is not authorized input at that point. The published constraints are
enforced for every field, so inputs the previous handler accepted silently or
mapped to defaults, such as `limit: 0`, an empty `search`, `status`, or `sort`,
`status: null`, and cursors over 512 characters, are now rejected.

## Prompts and completion

The server advertises the `prompts` and `completions` capabilities. MCP
completion can only target prompt or resource-template arguments, so the
`item_review_v1` prompt below exists to give argument autocompletion a real target.

| Surface | Scope | Safety |
|---|---|---|
| `item_review_v1` prompt | `resources:read` | read-only; requires `workspaceId`, `itemId`, and `status`; `prompts/get` checks the item through `items_get_v1`'s shared service |
| `completion/complete` for `status` | `resources:read` | read-only; returns `active` and/or `complete` by prefix |
| `completion/complete` for `itemId` | `resources:read` | read-only; scans the newest 100 items of the `workspaceId` named in the request context, optionally filtered by a valid `status` context value |

Completion is workspace-scoped through the caller's bearer token: an
unauthorized, unknown, or malformed `workspaceId` returns an empty value list
rather than an error, so missing and forbidden workspaces look the same.
Unknown prompts and unknown arguments return empty lists, and an `itemId`
request without a `workspaceId` context value also returns an empty list. Values are capped at 100 and `hasMore` is set when more
items exist than the single scanned page. Workspace IDs are not completed:
workspace enumeration remains browser-only.

Completion and prompt reads are not tool calls, so they write the
`mcp.request.called` record described under the security contract, not a
tool record. They return only data that `items_list_v1` or `items_get_v1` would return to
the same token.

WebMCP is separate from this server MCP endpoint. It is an optional,
feature-detected browser-tab enhancement that uses the live authenticated web
session and DOM to prepare narrow visible controls. It does not reuse bearer
credentials or change these scopes, tools, or transport. Server MCP remains
the persistent and authoritative automation surface.

## Resources

The endpoint also serves one read-only resource template:

| Template | MIME type | Scope | Safety |
|---|---|---|---|
| `dynamis://workspaces/{workspaceId}/items/{itemId}` | `application/json` | `resources:read` | read-only, workspace-scoped, no subscriptions |

Reads call the same workspace-scoped `items.Get` use case as `items_get_v1`
and return the item's JSON. Malformed URIs, unknown items, and items outside
the URI's workspace return `Resource not found`; a workspace the token cannot
read returns `access is denied` without item data. The template is named `item_v1`, following the
tool naming rule: a breaking URI, output, or side-effect change requires a new
name such as `item_v2`. The template is the only
resource: items are discovered through `items_list_v1`. Resource reads write an
`mcp.request.called` record, not a tool record.

## Discovery cache hints

`tools/list` returns `ttlMs: 300000` and `cacheScope: "public"`. The tool
catalog is identical for every authenticated principal and contains no
workspace data, so clients may reuse it for five minutes and shared caches may
keep it. The SDK's `SetCacheable` policy sets these fields only for that method.
The endpoint still requires a bearer token, and tool results are never cached
by this policy.

## Security contract

- Requests with `Origin` must exactly match `MCP_ALLOWED_ORIGINS`. Requests
  without `Origin` remain supported for non-browser clients.
- The server binds to loopback by default. Public binding is explicit through
  `HTTP_ADDRESS`; authentication remains mandatory.
- The endpoint is stateless and never issues `Mcp-Session-Id`.
- Bodies and structured results are limited to 1 MiB. Collection results are
  capped at 100 items.
- Update and delete require the client host to add
  `Mcp-Human-Approval: true` after human confirmation. Approval is not a tool
  argument and never replaces server authorization.
- Inputs are strict, bounded data. Internal errors become safe tool errors.
  Authentication and insufficient scope use HTTP `401` and `403`.

A receiving middleware on the SDK server audits, times, and recovers panics for
every method the SDK dispatches to the server. Each tool call writes an
`mcp.tool.called` record with principal, workspace, token/client, tool/version,
request ID, target, annotation, outcome, duration, and error-category fields.
Other dispatched methods, such as `tools/list` and `prompts/list`, write an
`mcp.request.called` record with the same safe fields plus `method`. Requests
that fail the SDK's own checks, such as unregistered tool names, are audited as
`tools/call` with `protocol_error`; the client-supplied name is never recorded.

Raw arguments, item titles, bearer values, panic values, and unrestricted
payloads are excluded. A panic becomes an `internal failure` response and an
`internal` audit record. If an audit write fails, the request returns
`internal failure` rather than a successful result.

The SDK rejects unknown method names before receiving middleware runs, so they
return JSON-RPC method-not-found and write no audit record. Client notifications
use a separate SDK path and are not audited.

Each accepted tool call writes an audit record containing safe principal,
workspace, token/client, tool/version, request ID, target, annotation, outcome,
duration, and error-category fields. Each prompt call writes an
`mcp.prompt.called` record with the same safe principal, request, client, and
outcome fields, a `workspace` target, and the prompt version.
Raw arguments, item titles, bearer values, and unrestricted payloads are
excluded.

## Protected-resource discovery

Discovery is optional and off by default. Set `MCP_OAUTH_AUTHORIZATION_SERVER`
to the authorization server issuer (HTTPS, or loopback HTTP for development),
and set `APP_PUBLIC_URL` to the public origin without a path. Startup fails if
either rule is violated. The resource identifier is `${APP_PUBLIC_URL}/mcp`,
derived from configuration and never from request `Host` headers.

When enabled:

- `GET /.well-known/oauth-protected-resource/mcp` serves unauthenticated
  RFC 9728 metadata: `resource`, `authorization_servers`, `scopes_supported`
  (`resources:read`, `resources:write`), and `bearer_methods_supported`
  (`header`). It accepts only `GET` and `OPTIONS`, and allows any origin to
  read it (`Access-Control-Allow-Origin: *`) because the document is public.
- A missing or invalid bearer returns `401` with
  `WWW-Authenticate: Bearer resource_metadata="<metadata URL>"`.
- A valid token without the tool's scope returns `403` with
  `WWW-Authenticate: Bearer error="insufficient_scope", resource_metadata="<metadata URL>"`.

When disabled, the route is absent and challenges carry no `WWW-Authenticate`
header, as before. Responses never include the submitted token, request
`Host`, or internal configuration, and the endpoint never issues a session ID.

Discovery does not change authentication by itself. Bearer API tokens are
always accepted. Access tokens from an external authorization server are
rejected; that path remains deferred in [capabilities](capabilities.md#deferred-capabilities).
Decision: [0007](decisions/0007-mcp-oauth-protected-resource-metadata.md).

## Built-in authorization server

Set `MCP_OAUTH_ENABLED=true` to make this application the authorization server
(decision [0008](decisions/0008-built-in-mcp-authorization-server.md)). It uses
`APP_PUBLIC_URL` as the issuer and cannot be combined with
`MCP_OAUTH_AUTHORIZATION_SERVER`.

- `POST /oauth/token` (form-encoded) handles `authorization_code` with PKCE S256
  and `refresh_token`. Access tokens last 15 minutes. Refresh tokens last 30
  days and rotate on every use. Reusing a rotated refresh token or a consumed
  code revokes the whole grant.
- `POST /oauth/revoke` (RFC 7009) revokes the grant that owns a token. Unknown
  tokens also return `200`.
- Token responses use `Cache-Control: no-store`. Errors use the OAuth format
  (`invalid_grant`, `invalid_client`, `unsupported_grant_type`), not Problem
  Details.
- Access is checked on every request. The consenting browser session must still
  be active, the user must still be a workspace member, and workspace MFA policy
  applies. Revoking the session, changing the password, removing the membership,
  or deleting the account ends access.
- Each grant is bound to one workspace, and its scopes are capped by the user's
  role when used.

Client registration (`/oauth/register`) and the consent page are not included in
this change; they follow in the next change. Until then, the endpoints above
work only for grants created by the identity service.

The protocol contract follows the upstream
[MCP specification](https://modelcontextprotocol.io/specification/2026-07-28)
and [official Go SDK](https://github.com/modelcontextprotocol/go-sdk).
