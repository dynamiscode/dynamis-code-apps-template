# MCP

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

WebMCP is separate from this server MCP endpoint. It is an optional,
feature-detected browser-tab enhancement that uses the live authenticated web
session and DOM to prepare narrow visible controls. It does not reuse bearer
credentials or change these scopes, tools, or transport. Server MCP remains
the persistent and authoritative automation surface.

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

Each accepted tool call writes an audit record containing safe principal,
workspace, token/client, tool/version, request ID, target, annotation, outcome,
duration, and error-category fields. Raw arguments, item titles, bearer values,
and unrestricted payloads are excluded.

The protocol contract follows the upstream
[MCP specification](https://modelcontextprotocol.io/specification/2026-07-28)
and [official Go SDK](https://github.com/modelcontextprotocol/go-sdk).
