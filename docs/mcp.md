# MCP (Agent profile)

This surface is included only when the application is generated with the
Agent profile. Core and Identity applications do not expose `/mcp`.

The authenticated Streamable HTTP endpoint is `POST /mcp`. It uses the
official Go SDK, the stateless MCP `2026-07-28` flow, and bounded legacy
`2025-11-25` initialization compatibility provided by that SDK.

Use the same bearer API tokens as REST. Discovery and read tools require
`resources:read`; mutation tools require `resources:write`. Every tool calls
a workspace-scoped application service. No tool accesses SQL, shell,
filesystem, code execution, or external networks.

## Tools

Names include their contract version. A breaking schema, scope, output, or side-effect change requires a new name. Without the Clients profile, no application tools are registered.

## Clients tools (Clients profile)

These tools exist only when the Clients profile is composed. Bootstrap passes
the Clients service to the MCP handler only then, so an application without
the profile lists no application tools. Each tool calls the shared Clients
service, which decides authorization. A refused call returns the safe
`access is denied` message, the same refusal REST answers with `403`. The one
exception is a workspace the token is not bound to: the Clients tools return the
not-found answer a missing record gets (`client not found`,
`custom field not found`, `choice option not found`, or `workspace not found` for
a collection call), the same as REST's `404`.

| Tools | Scope | Role, decided by the service |
|---|---|---|
| `clients_list_v1`, `clients_get_v1`, `clients_values_list_v1` | `resources:read` | any role |
| `client_fields_list_v1`, `client_field_options_list_v1` | `resources:read` | any role |
| `clients_create_v1`, `clients_update_v1`, `clients_archive_v1`, `clients_unarchive_v1`, `clients_value_set_v1`, `clients_value_clear_v1` | `resources:write` | members and above |
| `client_fields_define_v1`, `client_fields_archive_v1`, `client_fields_restore_v1`, `client_field_options_add_v1` | `resources:write` and `workspace:update` | owners and admins |
| `clients_delete_v1`, `client_fields_delete_v1`, `client_field_options_delete_v1` | `resources:write` and `workspace:update` | owners and admins; destructive, human approval required |

The `workspace:update` scope is a token scope, not only a role. A member cannot
hold it, so a member's token is refused for those rows. The permanent deletes
in this section prompt for client-host approval through the MRTR flow described below.

- Client and field short IDs are the only identifiers in inputs and outputs.
  Integer keys are refused by the schema. A malformed or other-type ID is not
  found, as REST answers it.
- `clients_list_v1` takes `archived` (`active`, `archived`, or `all`), one
  `sort` (`field` or `-field`), up to 10 `filters` of `{field, op, value}`
  (AND-combined), a limit of 1 to 100, and a 1-based page. The output carries
  `total` and the `page` returned.
- `clients_update_v1` needs `version` and at least one of `displayName`,
  `email`, `phone`, or `notes`. An empty `email`, `phone`, or `notes` clears
  it. A stale version returns `client version is stale`.
- Audit records use the client, field, or option target type with its integer
  key. Client values are never written to audit metadata.

## Prompts

No application prompts, resource templates, or completions are registered.
WebMCP is a separate, optional browser-tab enhancement; it does not change server MCP scopes or transport.

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
- Destructive Client, field, and option deletion requires client-host confirmation. On `2026-07-28` and later
  sessions, the first call returns an MRTR `input_required` result with one
  `human-approval` form elicitation (`confirm` boolean) and a `requestState`.
  The client retries the same arguments with `inputResponses` and the echoed
  state; nothing changes unless the response is `accept` with `confirm: true`
  and the state matches the arguments. Approval is never a tool argument and
  never replaces server authorization. Decision: [MRTR approvals](decisions/0007-mcp-mrtr-approvals.md).
- The `Mcp-Human-Approval` header is removed. Pre-`2026-07-28` protocol requests
  cannot confirm destructive deletion, because the stateless endpoint cannot deliver
  the server-initiated elicitation that the SDK would otherwise attempt.
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

Raw arguments, client field values, bearer values, panic values, and unrestricted
payloads are excluded. A panic becomes an `internal failure` response and an
`internal` audit record. If an audit write fails, the request returns
`internal failure` rather than a successful result.

The SDK rejects unknown method names before receiving middleware runs, so they
return JSON-RPC method-not-found and write no audit record. Client notifications
use a separate SDK path and are not audited.

Each accepted tool call writes an audit record containing safe principal,
workspace, credential (`credential_id`, the token's short ID), client,
tool/version, request ID, target, annotation, outcome, duration, and
error-category fields. Each prompt call writes an
`mcp.prompt.called` record with the same safe principal, request, client, and
outcome fields, a `workspace` target, and the prompt version.
Raw arguments, client field values, bearer values, and unrestricted payloads are
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

- `POST /oauth/register` (RFC 7591) registers public PKCE clients. Clients must
  use `token_endpoint_auth_method` `none`, grant types `authorization_code` and
  `refresh_token`, and response type `code`. Redirect URIs must be HTTPS or
  loopback HTTP and match exactly. Registrations are capped per instance
  (`429` when the cap is reached).
- `GET /.well-known/oauth-authorization-server` (RFC 8414) advertises the
  endpoints, `S256` PKCE, and `none` client authentication.
- `GET /oauth/authorize` is the browser consent page. It requires a browser
  session, redirects to sign-in when none exists, and asks the user to choose one
  workspace where they hold the requested scopes. The localized page shows the
  client name and requested resource permissions; approve and deny use ordinary
  CSRF-protected forms without automatic submission. `POST /oauth/authorize` checks
  the CSRF token, then approves or denies. Approval redirects to the client with a
  single-use code. Invalid client, redirect URI, or resource shows an error page
  rather than redirecting, because the target is untrusted until matched. The
  page is localized (English and Spanish) and is not exposed through WebMCP,
  which never handles consent, credentials, or tokens.

The protocol contract follows the upstream
[MCP specification](https://modelcontextprotocol.io/specification/2026-07-28)
and [official Go SDK](https://github.com/modelcontextprotocol/go-sdk).
