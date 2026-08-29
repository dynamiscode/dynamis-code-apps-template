# Changelog

## Unreleased

- Preserved forward upgrades for databases created from the pre-merge SCIM
  branch after migration numbering was reconciled with background jobs.

- Added REST-only SCIM 2.0 workspace provisioning for Users and role-mapped
  Groups, with one-time hashed dedicated credentials, bounded filtering and
  pagination, conditional PATCH/DELETE, safe errors, audit events, owner
  protection, passwordless verified-OIDC enrollment, workspace-scoped
  session/token revocation on deactivation, and read-only account profile
  fields, including order-independent primary email handling, filtered
  group-member removals, membership-aware group ETags, and retry-safe creates
  when `externalId` is omitted.

- Added the first durable background-jobs slice: a workspace-scoped,
  database-backed queue with lease recovery, idempotent webhook delivery,
  bounded retries, redacted status/metrics, and one worker loop per process.
- Added bounded public Item sharing with hashed opaque bearer tokens, seven-day
  default and 30-day maximum expiry, explicit write authorization, cascading
  invalidation, redacted public projections, access audits, rate limiting,
 safe browser headers, and ordinary-form CSRF-protected management. Files,
 REST, CLI, MCP, and WebMCP sharing remain out of scope.

- Added profile composition to application generation: Identity is required
  for Core, and selecting Agent includes its MCP and REST-only CLI surfaces;
  omitting Agent removes their implementation packages while preserving
  buildable Core/Identity applications.
- Added reusable repository hygiene files and safe support/triage guidance;
  generated applications rewrite repository and security links without
  retaining template maintainer details.
- Selected MIT as the project license, added SPDX and copyright metadata, and
  made generated applications carry MIT terms with application-owned copyright
  attribution and dependency notices.
- Made application generation require repository, security-reporting,
  maintainer, and profile metadata; generated applications no longer inherit
  template-owned repository links, ownership, or app-facing branding while
  `template.lock` preserves selected-profile and release provenance.
- Fixed account deletion for users who created items: items are retained and
  their creator reference becomes nullable instead of blocking user removal.
- Standardized item collection search, bounded query validation, query-bound
  cursors, and discoverable REST/OpenAPI conventions across REST, CLI, and MCP.
- Added bounded workspace item import from the versioned JSON export or strict
  `title,status` CSV, with authorization, transactional rollback, limits,
  audit outcomes, and deterministic `cmd/demo` seed support.
- Added Git-reviewed English and Spanish browser catalogs with account
  preference, explicit cookie, browser negotiation, workspace fallback,
  localized invitation emails, and language settings.
- Added migration 000006 and additive workspace export locale data; REST, CLI,
  MCP, WebMCP names, and machine-readable error contracts remain stable.
- Added account profile preferences, email verification, password reset/change,
  owner-safe account deletion, in-app notifications, recipient-scoped SSE, and
  notification retention. SMTP delivery remains synchronous and optional;
  retryable email delivery remains deferred.
- Fixed notification SSE initial/resync cursors, redacted notification events,
  and password-reset email locale resolution.
- Added OSS repository governance, dependency attribution, conduct, ownership,
  issue, and pull-request standards; project license selection remains pending.
- Added migration 000007 and workspace-scoped, encrypted-secret webhooks for
  item events, signed at-least-once delivery, bounded retries, and redacted
  delivery history.
- Added reproducible setup, module verification, secret scanning, pinned
  workflow checks, and version-pinned vulnerability scanning to local and CI
  gates.
- Added pinned CodeQL analysis for Go and JavaScript/TypeScript plus bounded
  fuzz smoke coverage for import and URL validators.
- Added REST consumer onboarding examples for bearer auth, pagination,
  idempotency, conditional writes, and stable Problem Details errors, plus a
  v1 item-surface compatibility test.

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
