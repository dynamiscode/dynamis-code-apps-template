# 0002: WebMCP Progressive Enhancement

- Status: accepted
- Date: 2026-08-26
- Owners: template maintainers

## Context

WebMCP is a proposed browser API with imperative and declarative variants.
The application already has server-rendered forms, CSRF protection, server
MCP, and browser security boundaries. Browser support is optional and can
change independently of the application release.

## Decision

Use the imperative WebMCP API only as a feature-detected, browser-tab-bound
enhancement for narrow non-secret workflows. Tools may prepare visible form
controls or the export screen's download link and focus the relevant control, but the user must
complete every state-changing submission through the existing flow. Do not
expose passwords, OIDC material, invitation or token secrets, sessions, CSRF
fields, hidden inputs, or operator lifecycle controls. Keep server MCP
persistent, bearer-authenticated, and authoritative. Unsupported browsers
must retain ordinary HTML navigation and forms.

## Consequences

The browser agent can assist with common workspace actions without a new
backend contract or dependency. Schemas and results stay bounded and safe,
but browser support must be tested conditionally and the human remains in the
mutation loop. Declarative WebMCP is deferred because existing forms contain
hidden security fields and repeated dynamic rows.

## Revisit when

The WebMCP specification stabilizes, browser support is available in supported
release browsers, or user research shows a need for additional safe tools.
