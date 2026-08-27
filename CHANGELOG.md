# Changelog

## Unreleased

- Added Git-reviewed English and Spanish browser catalogs with account
  preference, explicit cookie, browser negotiation, workspace fallback,
  localized invitation emails, and language settings.
- Added migration 000006 and additive workspace export locale data; REST, CLI,
  MCP, WebMCP names, and machine-readable error contracts remain stable.

- Local loopback browser setup without a token, with protected remote setup and
  environment bootstrap precedence.
- Browser workspace creation, member and ownership management, invitation
  acceptance, scoped token management, session management, OIDC login/linking,
  and workspace export.
- Added an authenticated workspace home at `/workspaces/{workspaceId}` with
  direct Items and Settings destinations; workspace navigation now preserves
  that home context and exposes Home above Items in the sidebar.
- Bearer-authenticated REST management endpoints for workspace identity
  resources, with one-time token secrets and optional STARTTLS SMTP invitation
  delivery.
- Optional progressive WebMCP browser tools for non-secret workspace, item,
  membership, revocation, session, and export preparation, with ordinary HTML
  fallback and conditional Selenium smoke coverage.
- Improved browser navigation spacing and current-page state, responsive
  controls, realtime connection feedback, and permanent-delete confirmation.
- Replaced the flat workspace link row with a responsive top-bar and sidebar
  shell, native workspace switcher, and account menu.
- Grouped members and invitations under Settings, kept Items and Settings at
  first level, and moved JSON export behind an export screen.
- Removed duplicate feature links from the top bar; the context-aware sidebar
  keeps Items at the top and anchors Settings at the bottom with its expanded
  member, token, and export links.
- Nested workspace management under the Settings route; Settings pages now show
  only their sub-navigation and provide a Back to home link to the current
  workspace.

## 0.1.0 - 2026-08-25
### Added

- Deployment-friendly first-owner bootstrap through environment variables,
  protected browser setup, and the existing CLI fallback.
- Portable SQLite/PostgreSQL foundation and explicit application composition.
- Local authentication, optional OIDC, workspace roles, secure sessions,
  invitations, scoped API tokens, and audit events.
- Bounded HTTP server, health contracts, OpenAPI 3.1 REST API, Problem Details,
  cursor pagination, conditional item updates, idempotency, and rate limits.
- Accessible server-rendered item flows, progressive HTMX fragments, permanent
  deletion, and authenticated database-backed SSE change delivery.
- Authenticated stateless MCP item tools with bounded contracts, approval and
  audit controls, plus the REST-only `appctl` client.
- OpenTelemetry traces and metrics, resource quotas, bounded workspace export,
  retention maintenance, interruption-safe migrations, and verified SQLite
  and PostgreSQL backup/restore paths.
- Pinned non-root container images, SQLite and PostgreSQL Compose profiles,
  automated delivery/security gates, semantic release artifacts, and
  non-overwriting application generation with `template.lock` provenance.
- Permanent maintenance, deployment, release, security, contribution, and
  agent-context documentation.
