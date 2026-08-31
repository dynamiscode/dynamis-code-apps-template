# 0006: Workspace Audit History Browser

- Status: accepted
- Date: 2026-08-29
- Owners: template maintainers

## Context

Audit events are append-only shared identity records. Owners and administrators
need a bounded browser view without creating an instance-wide console or
exposing the richer export record to ordinary page rendering.

## Decision

Add a read-only Settings → Audit history page at
`/workspaces/{workspaceId}/settings/audit`. Reuse the existing
`workspace:export` permission, so only current workspace owners and
administrators can access it. The shared identity use case returns at most the
100 newest events, ordered by occurrence time and event ID descending.

The browser projection contains event type, actor user ID, authentication
method, target type, action, outcome, and occurrence time. It omits workspace
and target IDs, request IDs, source addresses, and metadata. It uses the
existing server-rendered navigation, localization, `no-store` response
headers, and ordinary HTML table semantics. It has no REST, CLI, MCP, or
WebMCP surface and does not record a read audit event.

## Consequences

Workspace owners and administrators can inspect recent security and
administrative activity without receiving credentials, invitation values,
signed URLs, or arbitrary metadata. Older events remain available through the
authorized bounded workspace export and retention rules. The fixed limit
avoids an unbounded browser response; pagination and filtering remain out of
scope.

## Revisit when

Operators need a separately authorized audit role, a browser history beyond
the 100-event projection, event filtering or pagination for a demonstrated
correctness need, or a non-browser consumer requires a versioned audit
contract.
