# Capability and Conformance Ledger

This ledger records implemented conformance evidence and deferred triggers.

## Profile status

| Profile | Status | Evidence |
|---|---|---|
| Core | conforming | [Foundation through delivery evidence](#phase-07-evidence) |
| Identity | conforming | [Phase 02 evidence](#phase-02-evidence) |
| Agent | conforming | [Phase 05 evidence](#phase-05-evidence) |
| Production | not applicable | No deployment serves real users or durable data |

`Production` becomes applicable only when a deployment serves real users or
durable production data and records its deployment-specific evidence.

## Required standard groups

| Standard group | Lifecycle | Phase | Status | Evidence target |
|---|---|---:|---|---|
| Purpose, modular-monolith boundaries, scaling path | bootstrap, recurring | 01 | conforming | [Architecture](architecture.md), [composition test](../internal/bootstrap/app_test.go) |
| Optional WebMCP browser enhancement | triggered, recurring | 04, 07 | conforming | [WebMCP contract](web.md#webmcp-progressive-enhancement), [browser test](../internal/web/handler_test.go), [smoke](../scripts/webmcp-smoke.sh) |
| Browser baseline surfaces | bootstrap, recurring | 02, 04, 06 | conforming | [Web routes and controls](web.md), [browser tests](../internal/web/handler_test.go) |
| English/Spanish browser and invitation localization | bootstrap, recurring | 02, 04, 06 | conforming | [Localization contract](web.md#localization), [catalog tests](../internal/i18n/i18n_test.go), [identity tests](../internal/identity/locale_test.go), [web tests](../internal/web/handler_test.go) |
| REST and optional Agent MCP/remote CLI interfaces | bootstrap, recurring | 03-05 | conforming | REST: [OpenAPI](../api/openapi.json), [HTTP contracts](../internal/httpapi/handler_test.go); [MCP tests](../internal/mcpserver/server_test.go); [CLI tests](../internal/appctl/run_test.go) |
| Local authentication, OIDC, MFA, and passkey service | bootstrap, recurring | 02 | conforming | [Authentication](authentication.md), [MFA service](../internal/identity/mfa.go), [OIDC tests](../internal/identity/oidc_test.go), [configuration tests](../internal/platform/config/config_test.go) |
| Permissions, roles, workspaces, invitations, sessions, tokens, account lifecycle, preferences, and notifications service | bootstrap, recurring | 02 | conforming | [Authorization and lifecycle tests](../internal/identity/service_test.go), [account and notification tests](../internal/identity/account_notification_test.go), [PostgreSQL identity test](../internal/identity/postgres_test.go) |
| SQLite, PostgreSQL, migrations, and rolling compatibility | bootstrap, recurring | 01, 06 | conforming | [Database tests](../internal/platform/database/migrate_test.go), [PostgreSQL tests](../internal/platform/database/postgres_test.go), [upgrade procedure](operations.md#upgrades-and-alerts) |
| Data governance, portability, deletion, archive, restoration, account retention, and import | bootstrap, operational | 06, 07 | conforming | [Data lifecycle](data-lifecycle.md), [import/export tests](../internal/portability/service_test.go), [item lifecycle tests](../internal/items/service_test.go), [account tests](../internal/identity/account_notification_test.go), [retention tests](../internal/platform/maintenance/maintenance_test.go), [demo seed](../cmd/demo/main.go) |
| Configuration and secrets | bootstrap, recurring | 01 | conforming | [Configuration](configuration.md), [validation tests](../internal/platform/config/config_test.go), safe `.env.example` |
| HTTP limits, request IDs, timeouts, headers, and abuse controls | bootstrap, recurring | 03 | conforming | [HTTP component tests](../internal/httpapi/handler_test.go), [configuration](configuration.md) |
| Traces, metrics, logs, health, shutdown, and operational targets | bootstrap, operational | 06 | conforming | [Telemetry tests](../internal/platform/telemetry/telemetry_test.go), [operations and measurements](operations.md) |
| RFC 9457, collections, conditional writes, and idempotency | bootstrap, recurring | 03 | conforming | [API contract](api.md), [OpenAPI](../api/openapi.json), [HTTP contracts](../internal/httpapi/handler_test.go), [item service tests](../internal/items/service_test.go) |
| Contract lifecycle and deprecation | recurring | 03 | conforming | [Compatibility policy](api.md#compatibility), [generation drift test](../api/contract_test.go) |
| Long-running operations | bootstrap, triggered | 06 | not applicable | Current work is request-bounded; [trigger and rationale](data-lifecycle.md#portability) |
| Realtime delivery | bootstrap, recurring | 04 | conforming | [SSE contract](web.md#realtime-contract), [scope/reconnect/heartbeat/limit tests](../internal/web/handler_test.go) |
| Quotas and resource limits | bootstrap, operational | 06 | conforming | [Limit tests](../internal/httpapi/handler_test.go), [session tests](../internal/identity/service_test.go), [resource limits](operations.md#health-telemetry-and-limits) |
| Audit events | bootstrap, recurring | 02, 06 | conforming | [Identity audit tests](../internal/identity/service_test.go), [retention tests](../internal/platform/maintenance/maintenance_test.go), [access/deletion rules](data-lifecycle.md) |
| Workspace webhooks | triggered, recurring | 08 | conforming | [Decision](decisions/0004-webhooks.md), [REST contract](api.md#webhooks), [service tests](../internal/webhooks/service_test.go), [operations](operations.md#webhook-delivery) |
| Backup, restore, upgrades, RPO, and RTO | operational | 06 | conforming | [SQLite/PostgreSQL restore tests](../internal/platform/backup/backup_test.go), [operator procedures](operations.md#backup-and-restore) |
| WCAG 2.2 AA accessibility | bootstrap, recurring | 04, 07 | conforming | [Automated runner](../scripts/accessibility.mjs), [dated automated and manual evidence](accessibility.md) |
| Containers and deployment | bootstrap, operational | 07 | conforming | Pinned [image](../Dockerfile), [SQLite Compose](../compose.yaml), [PostgreSQL overlay](../compose.postgres.yaml), [smoke](../scripts/docker-smoke.sh), and [deployment contract](deployment.md) |
| CI, release security, SBOM, provenance, signatures, checksums | operational | 07 | conforming | [CI](../.github/workflows/ci.yml), [source security workflow](../.github/workflows/source-security.yml), [release workflow](../.github/workflows/release.yml), [dependency monitoring](../.github/dependabot.yml), [Scorecard](../.github/workflows/scorecard.yml), and [artifact verification](release.md) |
| MIT licensing, SPDX metadata, dependency attribution, and generated-repository governance | bootstrap, recurring | 07 | conforming | [Governance](governance.md), [SUPPORT](../SUPPORT.md), [LICENSE](../LICENSE), and [handoff test](../internal/conformance/handoff_test.go) |
| Documentation, context handoff, and sources of truth | bootstrap, recurring | 07 | conforming | [Router](README.md), [agent contract](../AGENTS.md), [handoff test](../internal/conformance/handoff_test.go), and permanent [skills](../.agents/skills/) |
| Testing strategy and complete template acceptance | recurring, operational | 01-07 | conforming | [Verification targets](../Makefile), [live surface smoke](../internal/bootstrap/agent_smoke_test.go), [accessibility smoke](../scripts/accessibility-smoke.sh), and [container smoke](../scripts/docker-smoke.sh) |

A group becomes `conforming` only when every applicable requirement in its
linked standard subsection passes and evidence is linked here. Use
`exception` only with an approved record under `docs/decisions/exceptions/`.

## Deferred capabilities

These remain out of the build plan until their trigger is demonstrated.

| Capability | State | Trigger |
|---|---|---|
| Webhooks | conforming | External consumers required delivery; accepted in [decision 0004](decisions/0004-webhooks.md) |
| Background jobs | deferred | Work must survive, retry, schedule, or outlive a request |
| Shared event broker | deferred | Cross-replica delivery cannot use the database safely |
| Object storage | deferred | Users upload or generate files |
| SCIM | deferred | Enterprise provisioning is required |
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

When a trigger is accepted, record the requirement and architecture decision,
then update implementation, contracts, tests, operations, and this ledger
together.

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

- atomic first-owner environment, local loopback browser-setup, protected remote browser-setup, and CLI smoke with no default credentials
- complete owner/admin/member/viewer, missing-membership, wrong-workspace,
  token-scope, role-change, credential-revocation, and final-owner matrix
- MFA enrollment, optional-user login, policy enforcement, challenge expiry,
  recovery, replay, and redacted audit coverage
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
- browser login, workspace creation, member/invitation/token/session controls,
  OIDC linking entry point, and export route use the shared services

## Phase 03 evidence

Verified 2026-08-25 with Go 1.27.0 and PostgreSQL 14.24:

- HTTP component and contract tests cover request correlation, security
  headers, time/body limits, health semantics, safe failures, workspace
  authorization, abuse controls, pagination, conditional writes, and
  idempotency
- item lifecycle and migrations pass on SQLite and isolated PostgreSQL
- `go generate ./api` followed by a clean generated contract diff
- bearer identity endpoints cover members, ownership, invitations, tokens, and
  sessions; workspace list/create remain intentionally browser-only
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
- browser surfaces cover workspace creation, baseline identity management,
  invitation registration/acceptance, one-time token display, sessions,
  security linking, and export download
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

## Phase 05 evidence

Verified 2026-08-25 with Go 1.27.0 and PostgreSQL 14.24:

- current stateless MCP discovery and bounded legacy initialization;
  authentication, read/write scopes, revoked tokens, exact Origins, no session
  IDs, stable schemas and annotations, approval signals, bounds, safe errors,
  wrong-workspace denial, and redacted tool audits
- REST-only CLI create/list/get/update/delete integration with JSON stream
  separation, exit statuses, bounded timeouts/responses, redirect rejection,
  and credential-safe errors
- live token creation, REST, CLI, MCP negotiation, and MCP tool call through
  one shared item use case
- production-source scans show no direct database or arbitrary shell path in
  MCP; the CLI dependency graph contains neither application services nor
  database packages
- `go test ./...`
- `go vet ./...`
- `go test -race ./...` on SQLite and with `POSTGRES_TEST_URL`
## Phase 06 evidence

Verified 2026-08-25 with Go 1.27.0, SQLite, PostgreSQL 14.24, and native
`pg_dump`/`pg_restore`:

- W3C-correlated server/client traces, metrics, structured logs, telemetry
  redaction, readiness, shutdown, request/session/stream/storage/export limits
- authorized versioned workspace export, wrong-workspace denial, explicit
  exclusions, audit outcomes, permanent item deletion, and documented import
  and identity-lifecycle boundaries
- browser export download is exposed while backup, restore, maintenance,
  import, audit administration, and deletion remain outside browser scope
- transaction-safe retention, interrupted migration rollback, checksummed
  SQLite snapshot and PostgreSQL dump, isolated known-record restore, and
  stale/corrupt evidence rejection
- measured SQLite server footprint and load with documented alert thresholds
- `go test ./...`, `go vet ./...`, `go test -race ./...`, reproducible
  `go generate ./api`, and a full race suite on isolated PostgreSQL

## Phase 07 evidence

Verified 2026-08-25 with Go 1.27.0, Docker Desktop, PostgreSQL 14.24,
Trivy 0.74.0, govulncheck 1.7.0, actionlint 1.7.12, Chrome, and Node 24:

- pinned minimal image built as UID/GID 65532 with executable health check,
  OCI metadata, 32.5 MB local runtime size, and persistent SQLite storage
- 2026-08-27 change verification passed bounded JSON/CSV item import,
  authorization, rollback, limits, audit outcomes, deterministic demo seeding,
  full Go tests, vet, race tests, command builds, and generated-app smoke;
  isolated PostgreSQL verification was unavailable in this environment
- clean Compose smoke proved health, environment first-owner bootstrap, browser login,
  useful item creation, restart, and persistent recovery; PostgreSQL overlay
  separately migrated, bootstrapped, restarted, and became ready
- full race suite passed on SQLite and isolated PostgreSQL, including
  interruption-safe migrations and known-record backup/restore
- live test proved browser login, authenticated REST, REST-only CLI, MCP
  initialization/tool call, and SSE notification through shared use cases
- generation replaced module, display, image, package, telemetry, API,
  configuration, documentation, repository, security, ownership, and branding
  identifiers; composed Core and required Identity, physically omitted the
  optional Agent implementation when unselected, and retained the Agent MCP
  and REST-only CLI surfaces when selected; copied MIT licensing and
  dependency attribution with application-owned copyright metadata; recorded
  selected profiles and required lock provenance; refused overwrite; and
  passed generated application suites
- source and runtime image scans found zero critical vulnerabilities;
  govulncheck found zero reachable vulnerabilities; CodeQL analyzes Go and
  JavaScript/TypeScript source; bounded fuzz smoke covers import and URL
  validators; SPDX image SBOM generated
- workflow YAML, pinned action commits, shell syntax, four-platform release
  builds, and SHA-256 checksum verification passed; the release workflow owns
  keyless signatures, provenance attestations, SBOM publication, and immutable
  remote artifacts
- automated accessibility smoke reported zero violations across login,
  workspace, items, and validation; dated manual evidence remains in
  [accessibility](accessibility.md)
- `make webmcp-smoke` passed ordinary fallback checks; native WebMCP assertions
  remain conditional on browser support and never fail solely because the API
  is absent
- documentation links, skill frontmatter/permanent routing, generated drift,
  semantic version, and temporary-source absence are enforced by the handoff
  test

## Phase 08 evidence

Webhook registration, dedicated workspace authorization, encrypted one-time
secrets, same-transaction item outbox rows, HMAC delivery headers, SSRF-safe
endpoint dialing, bounded retry/failure status, redacted delivery history,
retention pruning, and REST/OpenAPI behavior pass the focused webhook, item,
HTTP, configuration, maintenance, and migration tests. PostgreSQL execution and
Docker smoke remain part of the final verification ladder.

## Account deletion fix evidence

Verified 2026-08-28 with Go 1.27.0 and PostgreSQL 14.24:

- Account deletion retains creator-owned items, clears their nullable creator
  reference, and passes the focused SQLite regression.
- SQLite and isolated PostgreSQL migration, identity, item, and portability
  compatibility tests pass.
- `go test ./...`, `go vet ./...`, `go test -race ./...`, `make verify`, and
  `make docker-smoke` pass.
