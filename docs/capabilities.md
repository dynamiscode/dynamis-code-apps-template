# Capability and Conformance Ledger

This ledger reports current evidence while the template is under construction.

## Profile status

| Profile | Status | Evidence |
|---|---|---|
| Core | pending | Phases 01, 03, 04, 06, and 07 |
| Identity | conforming | [Phase 02 evidence](#phase-02-evidence) |
| Agent | pending | Phase 05 |
| Production | not applicable | No deployment serves real users or durable data |

`pending` is valid only during generation. `Core`, `Identity`, and `Agent`
must contain no pending groups before `STANDARDS.md` is deleted.

## Required standard groups

| Standard group | Lifecycle | Phase | Status | Evidence target |
|---|---|---:|---|---|
| Purpose, modular-monolith boundaries, scaling path | bootstrap, recurring | 01 | conforming | [Architecture](architecture.md), [composition test](../internal/bootstrap/app_test.go) |
 | Optional WebMCP browser enhancement | triggered, recurring | 04, 07 | pending | Browser registration, redaction, fallback, security-header, and smoke evidence |
 | Web, REST, MCP, and remote CLI interfaces | bootstrap, recurring | 03-05 | pending | Web: [component tests](../internal/web/handler_test.go); REST: [OpenAPI](../api/openapi.json), [HTTP contracts](../internal/httpapi/handler_test.go); MCP and CLI remain |
| Local authentication and OIDC | bootstrap, recurring | 02 | conforming | [Authentication](authentication.md), [OIDC tests](../internal/identity/oidc_test.go), [configuration tests](../internal/platform/config/config_test.go) |
| Permissions, roles, workspaces, invitations, sessions, and tokens | bootstrap, recurring | 02 | conforming | [Authorization and lifecycle tests](../internal/identity/service_test.go), [PostgreSQL identity test](../internal/identity/postgres_test.go) |
| SQLite, PostgreSQL, migrations, and rolling compatibility | bootstrap, recurring | 01, 06 | pending | Phase 01: [database implementation](../internal/platform/database/), real SQLite and PostgreSQL tests; rolling compatibility remains Phase 06 |
| Data governance, portability, deletion, archive, and restoration | bootstrap, operational | 06 | pending | Policies, use cases, and lifecycle tests |
| Configuration and secrets | bootstrap, recurring | 01 | conforming | [Configuration](configuration.md), [validation tests](../internal/platform/config/config_test.go), safe `.env.example` |
| HTTP limits, request IDs, timeouts, headers, and abuse controls | bootstrap, recurring | 03 | conforming | [HTTP component tests](../internal/httpapi/handler_test.go), [configuration](configuration.md) |
| Traces, metrics, logs, health, shutdown, and operational targets | bootstrap, operational | 06 | pending | Telemetry and lifecycle tests |
| RFC 9457, collections, conditional writes, and idempotency | bootstrap, recurring | 03 | conforming | [API contract](api.md), [OpenAPI](../api/openapi.json), [HTTP contracts](../internal/httpapi/handler_test.go), [item service tests](../internal/items/service_test.go) |
| Contract lifecycle and deprecation | recurring | 03 | conforming | [Compatibility policy](api.md#compatibility), [generation drift test](../api/contract_test.go) |
| Long-running operations | bootstrap, triggered | 06 | pending | Operation state and authorization tests |
| Realtime delivery | bootstrap, recurring | 04 | conforming | [SSE contract](web.md#realtime-contract), [scope/reconnect/heartbeat/limit tests](../internal/web/handler_test.go) |
| Quotas and resource limits | bootstrap, operational | 06 | pending | Limit enforcement and observability tests |
| Audit events | bootstrap, recurring | 02, 06 | pending | Redaction, append-only, and access tests |
| Backup, restore, upgrades, RPO, and RTO | operational | 06 | pending | Automated isolated restore evidence |
| WCAG 2.2 AA accessibility | bootstrap, recurring | 04, 07 | conforming | [Automated runner](../scripts/accessibility.mjs), [dated automated and manual evidence](accessibility.md) |
| Containers and deployment | bootstrap, operational | 07 | pending | Image and deployment smoke evidence |
| CI, release security, SBOM, provenance, signatures, checksums | operational | 07 | pending | Release workflow evidence |
| Documentation, context handoff, and sources of truth | bootstrap, recurring | 07 | pending | Link, drift, and context-routing checks |
| Testing strategy and complete template acceptance | recurring, operational | 01-07 | pending | Phase 01 Go, vet, race, SQLite, PostgreSQL, and startup checks pass; later phase gates remain |

A group becomes `conforming` only when every applicable requirement in its
linked standard subsection passes and evidence is linked here. Use
`exception` only with an approved record under `docs/decisions/exceptions/`.

## Deferred capabilities

These remain out of the build plan until their trigger is demonstrated.

| Capability | State | Trigger |
|---|---|---|
| Webhooks | deferred | External consumers require event delivery |
| Background jobs | deferred | Work must survive, retry, schedule, or outlive a request |
| Shared event broker | deferred | Cross-replica delivery cannot use the database safely |
| Object storage | deferred | Users upload or generate files |
| SCIM | deferred | Enterprise provisioning is required |
| MFA and passkeys | deferred | Identity ownership or measured risk requires stronger authentication |
| Feature flags | deferred | Staged rollout, kill switches, or targeting is required |
| AsyncAPI | deferred | A public asynchronous contract exists |
| A2A | deferred | The product hosts an independent communicating agent |
| GraphQL | deferred | Measured client-query needs exceed REST |
| gRPC | deferred | Typed streaming or throughput justifies another contract |
| Redis or cache | deferred | Measured latency or coordination cannot be solved by the app or database |
| Kubernetes | deferred | Deployment scale or organization policy requires orchestration |
| Billing | deferred | The product sells metered or subscription access |
| Search engine | deferred | Database search fails measured relevance or scale needs |
| Internal AI | deferred | Product behavior requires model-driven decisions |
| Workspace domains and routing | deferred | Users require workspace-specific domains or URLs |
| Public sharing or guest access | deferred | Resources must be reachable without normal membership |
| Fine-grained permissions | deferred | Baseline roles cannot express a measured requirement |

WebMCP is selected for this template as an optional progressive enhancement.
Its absence in a browser is not a release failure; its selected browser
surface is conforming only when the linked fallback, security, schema, focus,
and redaction evidence passes.

When a trigger is accepted, update `PLAN.md`, record the design decision, and
apply the minimum standard in `STANDARDS.md` Section 8 before implementation.

## Phase 01 evidence

Verified 2026-08-25 with Go 1.27.0 and PostgreSQL 14.24:

- `go test ./...`
- `go vet ./...`
- `go test -race ./...`
- full race suite with `POSTGRES_TEST_URL` against an isolated PostgreSQL
  database
- built server startup, SQLite file creation/migration, and SIGTERM shutdown

## Phase 02 evidence

Verified 2026-08-25 with Go 1.27.0 and PostgreSQL 14.24:

- atomic first-owner command smoke with no default password
- complete owner/admin/member/viewer, missing-membership, wrong-workspace,
  token-scope, role-change, credential-revocation, and final-owner matrix
- invitation expiry, duplicate prevention, resend rotation, acceptance,
  single-use, revocation, existing-account, and safe-error checks
- OIDC discovery plus state, browser session, S256 PKCE, nonce, provider,
  redirect, code, issuer, audience, signature, expiry, verified-email, replay,
  SSRF, and explicit-linking checks
- plaintext-secret absence and full session, invitation, token, audit, and OIDC
  transaction lifecycle on SQLite and isolated PostgreSQL
- `go test ./...`
- `go vet ./...`
- `go test -race ./...` with `POSTGRES_TEST_URL`

## Phase 03 evidence

Verified 2026-08-25 with Go 1.27.0 and PostgreSQL 14.24:

- HTTP component and contract tests cover request correlation, security
  headers, time/body limits, health semantics, safe failures, workspace
  authorization, abuse controls, pagination, conditional writes, and
  idempotency
- item lifecycle and migrations pass on SQLite and isolated PostgreSQL
- `go generate ./api` followed by a clean generated contract diff
- `go test ./...`
- `go vet ./...`
- `go test -race ./...` on SQLite and with `POSTGRES_TEST_URL`
- built server listener, live/ready requests, and SIGTERM graceful shutdown

## Phase 04 evidence

Verified 2026-08-25 with Go 1.27.0, PostgreSQL 14.24, HTMX 2.0.4,
axe-core 4.10.2, Chrome, and VoiceOver:

- item create, stable list, get, conditional update, permanent delete,
  workspace isolation, audit, redacted change events, and event retention on
  SQLite and isolated PostgreSQL
- full-page and HTMX fragment behavior, ordinary form fallback, session-bound
  CSRF, authorization, escaped output, validation, and safe errors
- SSE scope, initial/expired resync, valid reconnect replay, stable versioned
  payloads, heartbeat, lifetime, concurrency rejection, and redaction under
  the race detector
- zero axe violations on sign-in, workspace, items, and item-validation flows;
  keyboard, focus, 320 CSS-pixel reflow, reduced motion, accessibility-tree,
  and VoiceOver checks passed
- workspace home provides Items and Settings destinations; feature navigation keeps
  Home above Items in the workspace sidebar and anchors Settings
  at the bottom; nested Settings routes show only their members/invitations,
  API-token, and export sub-items; Items provides a Back to Workspaces link to
  the workspace selector and Settings provides a Back to home link to the current
  workspace home; current-page state and
  responsive keyboard/touch targets remain covered; destructive item actions have
  explicit confirmation and realtime status feedback
- optional WebMCP page markers, explicit tool schemas, secret/hidden-field
  exclusion, safe preparation, focus, and non-submission checks are covered by
  the browser contract test and conditional Selenium smoke
- `go test ./...`
- `go vet ./...`
- `go test -race ./...` on SQLite and with `POSTGRES_TEST_URL`
- reproducible `go generate ./api`
