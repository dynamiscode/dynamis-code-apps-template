# Capability and Conformance Ledger

This ledger records implemented conformance evidence and deferred triggers.

## Browser journey reconciliation (#454, 2026-11-17)

[Browser journey coverage reconciliation](web.md#browser-journey-coverage-reconciliation)
maps the route inventory's named route families and major states to Playwright
journeys, route tests, screenshot baselines, and explicit gaps. Representative
baselines cover workspace selection/home, Items, Clients, Members, Files,
notifications, and selected settings in desktop/mobile light/dark variants.
They do not capture every state or route.

Automated axe, keyboard/focus, accessible-name, reduced-motion, and 320px
reflow checks cover only their enumerated journeys. They do not establish full
WCAG conformance or replace assistive-technology and manual zoom checks. See
the [accessibility evidence](accessibility.md) for existing audit limits.

Dated evidence sections are append-only. New verification is recorded as a new
dated section; past dated sections are never rewritten, because each is a record
of what was verified on its date.

The [UI measurement method and asset inventory](ui-measurement.md) records the
baseline environment, screen inventory, home observation and remaining screen
measurements for issue #439. [Administration evidence](ui-measurement-admin.md)
records settings and data-management observations; [identity evidence](ui-measurement-identity.md)
records account, authentication and public-screen observations. These establish
no performance budgets and do not claim screen measurements are complete.
The [workspace journey evidence](ui-measurement-workspace.md) records request,
transfer, timing, mutation, loading and failure observations for issue #447,
with unavailable metrics and states marked explicitly.
The [reconciliation and budget recommendation](ui-measurement-reconciliation-450.md)
summarizes measured results, limitations, unverified surfaces, and budget
decisions; it does not claim implementation of proposed fixes.

The [administration measurement evidence](ui-measurement-admin.md) records the
bounded screen and state measurements for issue #448. Metrics the harness cannot
isolate remain explicitly unavailable; no budgets or performance fixes are claimed.

The [account, authentication, and public measurement evidence](ui-measurement-identity.md)
is partial for issue #449; invalid-login and password-recovery feedback and
notification SSE connection are measured. MFA, MCP consent, valid public
shares, and remaining interaction states remain unmeasured.

## Profile status

| Profile | Status | Evidence |
|---|---|---|
| Core | conforming | [Foundation through delivery evidence](#phase-07-evidence) |
| Identity | conforming | [Phase 02 evidence](#phase-02-evidence) |
| Agent | conforming | [Phase 05 evidence](#phase-05-evidence) |
| Files | conforming | [Files evidence](#files-evidence) |
| Clients | partial | Create and edit clients over REST and web: [service tests](../internal/clients/service_test.go), [PostgreSQL lifecycle](../internal/clients/postgres_test.go), [REST tests](../internal/httpapi/clients_test.go), [web tests](../internal/web/clients_test.go), template-init generation tests (`cmd/template-init`). The list uses `clients.templ`, loads local HTMX and `app.js`, and refreshes its list fragment from the `client.changed` and `resync` SSE events under `ResourcesRead`; it defers replacement while forms are focused, dirty, or submitting: [#415 evidence](clients-list-evidence-415.md), [browser coverage](../scripts/playwright/clients-list.spec.mjs), [listener tests](../internal/web/assets/realtime-lists.test.mjs). Define, archive, and restore text, number, date, and yes/no custom fields over REST and web: [field service tests](../internal/clients/fields_test.go), [REST tests](../internal/httpapi/client_fields_test.go), [web tests](../internal/web/client_fields_test.go), [slot-index tests](../internal/platform/database/client_slot_indexes_test.go). Set and clear text, number, date, and yes/no values with the field type lock: [value service tests](../internal/clients/values_test.go), [typed value tests](../internal/clients/typed_values_test.go), [PostgreSQL values](../internal/clients/postgres_test.go), [REST tests](../internal/httpapi/client_values_test.go), [web tests](../internal/web/client_values_test.go). Client changes have a workspace SSE feed and list fragment: [change feed tests](../internal/clients/events_test.go), [PostgreSQL feed check](../internal/clients/postgres_test.go), [stream and fragment tests](../internal/web/clients_events_test.go), [listener unit tests](../internal/web/assets/realtime-lists.test.mjs). Single-choice fields with up to 100 options per field over REST and web, with option deletion clearing values: [option service tests](../internal/clients/options_test.go), [option ID tests](../internal/clients/option_id_test.go), [PostgreSQL options](../internal/clients/postgres_test.go), [REST tests](../internal/httpapi/client_options_test.go), [web tests](../internal/web/client_options_test.go). Permanent deletion of an archived custom field over REST and web, clearing its values and choice options in one transaction: [field delete service tests](../internal/clients/field_delete_test.go), [REST tests](../internal/httpapi/client_field_delete_test.go), [PostgreSQL lifecycle](../internal/clients/postgres_test.go). Client archive, unarchive, and permanent client delete over REST and web, with the archived list selector: [lifecycle service tests](../internal/clients/lifecycle_test.go), [PostgreSQL archive and delete](../internal/clients/postgres_test.go), [REST tests](../internal/httpapi/client_lifecycle_test.go), [web tests](../internal/web/client_lifecycle_test.go). Client webhook events `client.created`, `client.updated`, `client.archived`, and `client.deleted` through the existing outbox, written in the change's transaction, with custom value set and clear emitting `client.updated`: [SQLite and PostgreSQL outbox tests](../internal/clients/webhook_events_test.go), [subscription test](../internal/webhooks/client_events_test.go). Client export, MCP, and `appctl` for archive and delete not implemented |
| Clients shared list view | partial | Per-user columns and custom Client values, responsive table/card markup, sort/filter/pagination URL state, explicit-empty and typed timestamp filters, filter chips, reset actions, and viewer isolation: [web behavior tests](../internal/web/clients_test.go), [typed query tests](../internal/clients/query_test.go), [REST compatibility tests](../internal/httpapi/clients_test.go), [preference service tests](../internal/listprefs/service_test.go), and generated-profile checks (`cmd/template-init`). Required 1,000/10,000-row latency benchmarks and VoiceOver walkthrough remain pending. |
| Items shared list view | partial | Per-user title/status visibility, responsive shared table/card layout, title search and status filters, stable creation-time ordering, cursor pagination URL state, query-preserving realtime refresh, and inline editing with hidden columns: [web behavior test](../internal/web/items_list_test.go), [Items service tests](../internal/items/service_test.go), [realtime browser test](../internal/web/assets/realtime-lists.test.mjs), [preference service tests](../internal/listprefs/service_test.go), and generated-profile checks (`cmd/template-init`). Required 1,000/10,000-row latency benchmarks and VoiceOver walkthrough remain pending. |
| Members shared list view | partial | Per-user email/role/created visibility, responsive shared table/card layout, email and role filters, stable sorting, full results by default, opt-in cursor paging in web and REST, separate CSRF-protected role/removal/ownership forms, explicit no-JavaScript confirmations, and user-controlled WebMCP actions: [web behavior tests](../internal/web/members_list_test.go), [mutation tests](../internal/web/members_role_test.go), [member browser coverage](../scripts/playwright/members.spec.mjs), [REST behavior tests](../internal/httpapi/handler_test.go), and [preference service tests](../internal/listprefs/service_test.go). Full suite and browser/accessibility checks passed; required 1,000/10,000-row latency benchmarks and VoiceOver walkthrough remain pending. |
| Files shared list view | partial | Saved columns, typed filters, stable sorting, keyset pagination, and shared responsive table/card rendering: [Files web tests](../internal/web/files_test.go), service tests and generated-profile checks. Required 1,000/10,000-row latency benchmarks and VoiceOver walkthrough remain pending. |
| Production | not applicable | No deployment serves real users or durable data |

`Production` becomes applicable only when a deployment serves real users or
durable production data and records its deployment-specific evidence.

## Required standard groups

| Standard group | Lifecycle | Phase | Status | Evidence target |
|---|---|---:|---|---|
| Purpose, modular-monolith boundaries, scaling path | bootstrap, recurring | 01 | conforming | [Architecture](architecture.md), [composition test](../internal/bootstrap/app_test.go) |
| Optional WebMCP browser enhancement | triggered, recurring | 04, 07 | conforming | [WebMCP contract](web.md#webmcp-progressive-enhancement), [browser test](../internal/web/handler_test.go), [smoke](../scripts/webmcp-smoke.sh) |
| Browser baseline surfaces | bootstrap, recurring | 02, 04, 06 | conforming | [Web routes and controls](web.md), [route handler tests](../internal/web/handler_test.go), [members and invitation browser journeys](../scripts/playwright/members.spec.mjs), [workspace administration and data journeys](../scripts/playwright/workspace-admin-data.spec.mjs) |
| English/Spanish browser and invitation localization | bootstrap, recurring | 02, 04, 06 | conforming | [Localization contract](web.md#localization), [catalog tests](../internal/i18n/i18n_test.go), [identity tests](../internal/identity/locale_test.go), [web tests](../internal/web/handler_test.go) |
| Light, dark, and system appearance | bootstrap, recurring | 02, 04, 06 | conforming | [Theme contract](web.md#theme), [theme tests](../internal/web/theme_test.go), [web tests](../internal/web/handler_test.go) |
| REST and optional Agent MCP/remote CLI interfaces | bootstrap, recurring | 03-05, 09 | conforming | REST: [OpenAPI](../api/openapi.json), [HTTP contracts](../internal/httpapi/handler_test.go), [SCIM contracts](../internal/httpapi/scim_test.go); [MCP tests](../internal/mcpserver/server_test.go); [CLI tests](../internal/appctl/run_test.go) |
| Local authentication, OIDC, MFA, and passkey service | bootstrap, recurring | 02 | conforming | [Authentication](authentication.md), [first-run browser setup tests](../internal/web/handler_test.go), [MFA service](../internal/identity/mfa.go), [OIDC tests](../internal/identity/oidc_test.go), [configuration tests](../internal/platform/config/config_test.go) |
| Optional MCP OAuth protected-resource metadata | bootstrap, recurring | 05 | conforming, off by default; trigger accepted in [decision 0007](decisions/0007-mcp-oauth-protected-resource-metadata.md) | [MCP discovery](mcp.md#protected-resource-discovery), `internal/mcpserver/oauth_test.go`, `internal/bootstrap/agent_smoke_test.go` (Agent profile only), [configuration tests](../internal/platform/config/config_test.go) |
| Built-in MCP OAuth authorization server | bootstrap, recurring | 05 | implemented behind `MCP_OAUTH_ENABLED`; expired-record pruning and accessibility-smoke evidence pending | [decision 0008](decisions/0008-built-in-mcp-authorization-server.md), [MCP OAuth](mcp.md#built-in-authorization-server), `internal/identity/mcp_oauth_test.go`, `internal/mcpserver/oauth_token_test.go`, [configuration tests](../internal/platform/config/config_test.go) |
| Permissions, roles, workspaces, invitations, sessions, tokens, SCIM provisioning, account lifecycle, preferences, and notifications service | bootstrap, recurring | 02, 09 | conforming | [Authorization and lifecycle tests](../internal/identity/service_test.go), [SCIM lifecycle tests](../internal/identity/scim_test.go), [account and notification tests](../internal/identity/account_notification_test.go), [PostgreSQL identity test](../internal/identity/postgres_test.go) |
| SQLite, PostgreSQL, migrations, and rolling compatibility | bootstrap, recurring | 01, 06 | conforming | [Database tests](../internal/platform/database/migrate_test.go), [PostgreSQL tests](../internal/platform/database/postgres_test.go), [upgrade procedure](operations.md#upgrades-and-alerts) |
| Data governance, portability, deletion, archive, restoration, account retention, and import | bootstrap, operational | 06, 07 | conforming | [Data lifecycle](data-lifecycle.md), [import/export service tests](../internal/portability/service_test.go), [web import safeguards](../internal/web/handler_test.go), [import/export browser journey](../scripts/playwright/workspace-admin-data.spec.mjs), [item lifecycle tests](../internal/items/service_test.go), [account tests](../internal/identity/account_notification_test.go), [retention tests](../internal/platform/maintenance/maintenance_test.go), [demo seed](../cmd/demo/main.go) |
| Per-user workspace list-column visibility preferences | triggered, recurring | 04 | partial | [Preference service tests](../internal/listprefs/service_test.go), [migration](../internal/platform/database/migrations/000026_list_column_preferences.sql); list-specific defaults and UI integration are delivered with each list migration. |
| Configuration and secrets | bootstrap, recurring | 01 | conforming | [Configuration](configuration.md), [validation tests](../internal/platform/config/config_test.go), safe `.env.example` |
| HTTP limits, request IDs, timeouts, headers, and abuse controls | bootstrap, recurring | 03 | conforming | [HTTP component tests](../internal/httpapi/handler_test.go), [configuration](configuration.md) |
| Traces, metrics, logs, health, shutdown, and operational targets | bootstrap, operational | 06 | conforming | [Telemetry tests](../internal/platform/telemetry/telemetry_test.go), [operations and measurements](operations.md) |
| RFC 9457, collections, conditional writes, and idempotency | bootstrap, recurring | 03 | conforming | [API contract](api.md), [OpenAPI](../api/openapi.json), [HTTP contracts](../internal/httpapi/handler_test.go), [item service tests](../internal/items/service_test.go) |
| Contract lifecycle and deprecation | recurring | 03 | conforming | [Compatibility policy](api.md#compatibility), [generation drift test](../api/contract_test.go) |
| Long-running operations | bootstrap, triggered | 06 | not applicable | Current work is request-bounded; [trigger and rationale](data-lifecycle.md#portability) |
| Realtime delivery | bootstrap, recurring | 04 | conforming | [SSE contract](web.md#realtime-contract), [scope/reconnect/heartbeat/limit tests](../internal/web/handler_test.go) |
| Quotas and resource limits | bootstrap, operational | 06 | conforming | [Limit tests](../internal/httpapi/handler_test.go), [session tests](../internal/identity/service_test.go), [resource limits](operations.md#health-telemetry-and-limits) |
| Audit events | bootstrap, recurring | 02, 04, 06 | conforming | [Identity audit tests](../internal/identity/audit_test.go), [browser audit tests](../internal/web/handler_test.go), [retention tests](../internal/platform/maintenance/maintenance_test.go), [access/deletion rules](data-lifecycle.md), [decision](decisions/0006-workspace-audit-history.md) |
| Workspace webhooks | triggered, recurring | 08-09 | conforming | [Decision](decisions/0004-webhooks.md), [REST contract](api.md#webhooks), [browser contract](web.md), [service tests](../internal/webhooks/service_test.go), [handler lifecycle and delivery-redaction tests](../internal/web/webhooks_test.go), [no-key browser guard](../scripts/playwright/workspace-admin-data.spec.mjs), [operations](operations.md#webhook-delivery) |
| Background jobs | triggered, recurring | 09 | conforming | [Decision](decisions/0005-background-jobs.md), [queue tests](../internal/jobs/queue_test.go), [migration](../internal/platform/database/migrations/000010_background_jobs.sql), [operations](operations.md#health-telemetry-and-limits) |
| Bounded public Item sharing | triggered, recurring | 09 | conforming | [Decision](decisions/0005-public-sharing.md), [web contract](web.md), [sharing tests](../internal/sharing/service_test.go), [browser tests](../internal/web/handler_test.go), [lifecycle](data-lifecycle.md) |
| Backup, restore, upgrades, RPO, and RTO | operational | 06 | conforming | [SQLite/PostgreSQL restore tests](../internal/platform/backup/backup_test.go), [operator procedures](operations.md#backup-and-restore) |
| WCAG 2.2 AA accessibility | bootstrap, recurring | 04, 07 | conforming | [Automated runner](../scripts/accessibility.mjs), [dated automated and manual evidence](accessibility.md), including workspace admin and data routes |
| Containers and deployment | bootstrap, operational | 07 | conforming | Pinned [image](../Dockerfile), [SQLite Compose](../compose.yaml), [PostgreSQL overlay](../compose.postgres.yaml), [smoke](../scripts/docker-smoke.sh), and [deployment contract](deployment.md) |
| CI, source security, container SBOM | operational | 07 | conforming | [CI](../.github/workflows/ci.yml), [source security workflow](../.github/workflows/source-security.yml), [dependency monitoring](../.github/dependabot.yml), [Scorecard](../.github/workflows/scorecard.yml) |
| MIT licensing, SPDX metadata, dependency attribution, and generated-repository governance | bootstrap, recurring | 07 | conforming | [Governance](governance.md), [SUPPORT](../SUPPORT.md), [LICENSE](../LICENSE), and [handoff test](../internal/conformance/handoff_test.go) |
| Documentation, context handoff, and sources of truth | bootstrap, recurring | 07 | conforming | [Router](README.md), [agent contract](../AGENTS.md), [handoff test](../internal/conformance/handoff_test.go), and permanent [skills](../.agents/skills/) |
| Testing strategy and complete template acceptance | recurring, operational | 01-09 | conforming | [Verification targets](../Makefile), [live surface smoke](../internal/bootstrap/agent_smoke_test.go), [queue tests](../internal/jobs/queue_test.go), [accessibility smoke](../scripts/accessibility-smoke.sh), and [container smoke](../scripts/docker-smoke.sh) |

A group becomes `conforming` only when every applicable requirement in its
linked standard subsection passes and evidence is linked here. Use
`exception` only with an approved record under `docs/decisions/exceptions/`.

## Deferred capabilities

These remain out of the build plan until their trigger is demonstrated.

| Capability | State | Trigger |
|---|---|---|
| Versioned release publication | deferred | Re-enable only after #355 controls and #357 merge-authorized publication evidence are complete and reviewed |
| Webhooks | conforming | External consumers required delivery; accepted in [decision 0004](decisions/0004-webhooks.md) |
| Background jobs | conforming | Bounded webhook delivery requires durable retry and lease ownership; see [phase 09 evidence](#phase-09-evidence) |
| Shared event broker | deferred | Cross-replica delivery cannot use the database safely |
| Object storage | conforming | Files profile accepted; [Files evidence](#files-evidence); [S3 conformance evidence](#object-storage-s3-conformance-evidence) |
| SCIM | conforming | Enterprise provisioning accepted and implemented in Phase 09 |
| MFA and passkeys | conforming | MFA and passkey identity flows accepted and implemented in Phase 02 |
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
| Public sharing or guest access | conforming for bounded Item browser links | Files, public APIs, guest collaboration, or other resource types require a separate contract |
| Fine-grained permissions | deferred | Baseline roles cannot express a measured requirement |
| External-issuer OAuth access tokens for MCP | deferred | An accepted requirement needs `/mcp` to accept access tokens from an external authorization server; see [decision 0007](decisions/0007-mcp-oauth-protected-resource-metadata.md) |
| Email providers | deferred | A workspace needs an HTTP email provider beyond Resend and Postmark. Per-workspace SMTP (issue 203), Resend (issue 204), and Postmark (issue 205) are implemented; each further HTTP provider needs its own contract and evidence. Architecture is accepted in [decision 0009](decisions/0009-workspace-email-settings.md). |
| Encryption key rotation command | deferred | Stored provider credentials must be re-encrypted under a new key after a compromise or policy change. Design is accepted in [decision 0009](decisions/0009-workspace-email-settings.md); no rotation command exists yet. |

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
- browser surfaces cover workspace creation in multi mode, baseline identity management,
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
  import, audit administration, and deletion remain outside browser scope;
  owners/admins have a separate redacted read-only audit history page
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
retention pruning, REST/OpenAPI behavior, and the CSRF-protected browser
management surface pass the focused webhook, item, browser, HTTP, configuration,
maintenance, and migration tests. PostgreSQL execution and Docker smoke remain
part of the final verification ladder.

## Workspace email evidence

Workspace SMTP settings seal the password under `ENCRYPTION_KEY` and record
`secret_key_version` (migration 000018). Admin-only management, a failed login
test blocking save, blank-password retention, invitation routing to the
workspace sender with instance fallback, and env-only account email are covered
by `TestWorkspaceEmailSettingsAreAdminOnlyAndLoginTested` in `internal/web`, the
`internal/workspaceemail` service tests, and the display-name test in
`internal/platform/mail`. Not covered by automated tests: a delivered message
through a live STARTTLS server. Verify that manually against a real provider
before release.

## Workspace Resend evidence

Workspace email can use Resend (`provider = resend`, migration 000019). The
save path runs a read-only `GET /domains` check with the submitted key and stores
nothing if it fails. Sending uses `POST /emails` over HTTPS with the sealed key.
Evidence: `internal/platform/mail` tests against a fake HTTP server (bearer
auth, payload shape, rejected key, no response body echoed), the
`internal/workspaceemail` tests (bad key blocks save, sealed key, blank-key reuse,
invitation delivered through Resend), and `TestResendSettingsRequireAPIKeyCheckBeforeSaving`
in `internal/web`. Not covered by automated tests: a real Resend account. Verify
a sent invitation against Resend before release.

## Workspace Postmark evidence

Workspace email can use Postmark (`provider = postmark`, migrations 000019 and
000021). The save path runs a read-only `GET /server` check with the submitted
Server API token and a read-only `GET /domains` check with the Account API token,
and stores nothing if either fails. Sending uses `POST /email` over HTTPS with the
sealed Server API token. The Account API token is sealed in its own column with
its own key version. Evidence: `internal/platform/mail` tests against a fake HTTP
server (server token header, payload shape, rejected token, no response body
echoed, read-only checks), the `internal/workspaceemail` tests (bad server or
account token blocks save, sealed tokens, blank-token reuse, invitation delivered
through Postmark), and `TestPostmarkSettingsRequireTokenChecksBeforeSaving` in
`internal/web`. Not covered by automated tests: a real Postmark account. Verify a
sent invitation against Postmark before release.

Surfaces omitted: REST/OpenAPI (sender settings are browser-only; no API client
needs them yet), CLI/MCP (no automation needs to change sender settings), and
realtime (no client needs change delivery for sender settings). The browser
surface is the workspace email settings form, with CSRF and ordinary form
submission.

## General encryption key evidence

Webhook secrets seal with `ENCRYPTION_KEY` and record `secret_key_version`
(migration 000017). `WEBHOOK_ENCRYPTION_KEY` is key version 1 and still opens
rows sealed before `ENCRYPTION_KEY` was set. Evidence: the legacy-ciphertext
vector test in `internal/platform/secretbox`, the key-selection matrix in
`internal/platform/config`, `TestLegacyWebhookSecretsDeliverAfterGeneralKeyIsAdded`
in `internal/webhooks`, and `TestPostgresMigrations` in `internal/platform/database`,
which applies every embedded migration including 000017. A manual startup with
no key set logged the warning and reached the listener (the smoke was not
committed as a test). Key rotation remains deferred.

## Phase 09 evidence

SCIM 2.0 Users and role Groups are REST-only and workspace-scoped. The browser
Settings → Provisioning page manages only the dedicated workspace credential;
it does not expose a browser SCIM directory or WebMCP credential tool. SQLite
tests cover dedicated-token separation, one-time secret storage, stable
external IDs, normalized email identity, passwordless verified-OIDC enrollment,
idempotent create, exact filtering, pagination, PATCH/DELETE ETags, group role
mapping, workspace-scoped deactivation revocation, safe errors, audit redaction,
and final-owner protection. PostgreSQL identity
and migration checks cover migrations 000013-000014 and the
provisioning/deactivation path when `POSTGRES_TEST_URL` is configured. Browser
tests cover owner/admin authorization, workspace isolation, CSRF, endpoint
rendering, one-time secret response behavior, revocation, no-store headers,
localization, and WebMCP exclusion.

## Workspace audit history evidence

Verified 2026-08-29 with SQLite:

- owners and admins can read only their workspace's latest 100 audit events
  through the shared identity use case and Settings page; members, viewers,
  missing memberships, and cross-workspace requests are denied
- the browser projection omits metadata, workspace and target IDs, request IDs,
  and source addresses; no credentials, invitation values, signed URLs, or
  WebMCP controls are rendered
- fixed ordering, localization parity, semantic table markup, `no-store`
  caching, ordinary navigation, and focused identity/browser regression tests
  pass; REST, CLI, MCP, pagination, and filtering remain intentionally omitted

Bounded Item sharing passes hashed-token storage, default and maximum expiry,
explicit write authorization, safe title/status-only public projection,
revocation, item-deletion invalidation, access outcome audits, browser CSRF,
public response headers, and public-token path redaction tests. REST, CLI, MCP,
WebMCP, file sharing, public writes, search, listing, and indexing remain
intentionally omitted by the accepted [decision](decisions/0005-public-sharing.md).

Verified 2026-08-28 with SQLite and Docker smoke. The database-backed worker
queue persists workspace-scoped webhook jobs in the same transaction as item
outbox rows, deduplicates enqueue keys, claims with lease tokens, reclaims
expired leases, records bounded attempt/status/timestamps, retries up to five
times, and stores only redacted failure categories. Focused queue, webhook,
maintenance, migration, telemetry, and isolated changed-package race tests
pass. PostgreSQL tests were skipped because `POSTGRES_TEST_URL` was unset;
the final full `make verify` race phase was resource-contended by concurrent
test processes and failed only in the existing live bootstrap smoke timeout.

## Account deletion fix evidence

Verified 2026-08-28 with Go 1.27.0 and PostgreSQL 14.24:

- Account deletion retains creator-owned items, clears their nullable creator
  reference, and passes the focused SQLite regression.
- SQLite and isolated PostgreSQL migration, identity, item, and portability
  compatibility tests pass.
- `go test ./...`, `go vet ./...`, `go test -race ./...`, `make verify`, and
  `make docker-smoke` pass.

## Settings import evidence

Verified 2026-08-29 with focused Go tests on SQLite:

- owners and admins reach the server-rendered Settings import form; viewers
  are denied by shared workspace authorization
- one bounded `.json` export or strict UTF-8 `title,status` CSV upload requires
  session CSRF and explicit bulk-mutation confirmation; invalid format,
  content, size, quota, rollback, stateless query-param success, Spanish localization,
  safe output, ordinary-form fallback, and multi-instance stateless redirects
  are covered by browser handler tests
- import remains synchronous and atomic through the existing portability use
  case; REST/OpenAPI, MCP, remote CLI, and WebMCP contracts remain unchanged
- focused web, portability, localization, and template-generation tests pass;
  full-suite listener-dependent tests require execution outside the restricted
  sandbox

## Workspace short ID evidence

Verified 2026-11-11 with Go 1.27.4. Issue #189 moves workspace keys to integers
and exposes only the ADR 0009 short ID. Implemented and verified as recorded above; the
exception is [2026-11-11-workspace-short-id-migration-rewrite](decisions/exceptions/2026-11-11-workspace-short-id-migration-rewrite.md).

## Sharing, SCIM, and MCP OAuth short ID evidence

Issue #194 moves public link, SCIM token, OAuth client, and OAuth grant keys to
integers and exposes only the ADR 0009 short ID, using the type codes
`public_link`, `scim_token`, `oauth_client`, and `oauth_grant`. Public-share
tokens, SCIM secrets, OAuth codes, access and refresh tokens, and OAuth client
IDs stay random and are not encoded. A malformed or foreign-type public link ID
returns the same not-found as a missing link. The exception is
[2026-11-11-sharing-scim-oauth-short-id-migration-rewrite](decisions/exceptions/2026-11-11-sharing-scim-oauth-short-id-migration-rewrite.md).
Verified 2026-11-11 with Go 1.27.4 and PostgreSQL 14.24: focused tests for the codec, sharing
revoke, SCIM token audit targets, and OAuth grant and client audit targets pass; `make test`
passes on SQLite; `go test -p 1 -count=1 ./...` passes on a fresh PostgreSQL 14.24 container;
`make lint` and `go generate ./api` show no drift.


## Notification short ID evidence

Issue #245 moves notification keys to integers and exposes only the ADR 0009 short
ID in web JSON, SSE events, and the `after` cursor, using the type code
`notification`. A malformed notification cursor resyncs, and a malformed mark-read ID
returns the same denial as an unknown one. Email verification and password reset rows
keep text keys. The exception is
[2026-11-12-notification-account-token-short-id-migration-rewrite](decisions/exceptions/2026-11-12-notification-account-token-short-id-migration-rewrite.md).
Verified 2026-11-12 with Go 1.27.4: focused tests for the type-code registry and notification
cursor and mark-read rejection pass on SQLite and PostgreSQL 14.24; `make test` and `make lint`
pass on SQLite; `go test -p 1 -count=1 ./internal/identity/... ./internal/files/...` passes on a
fresh PostgreSQL 14.24 container; `go generate ./api` shows no drift.

## Authentication journey evidence

Issues #419 and #420 use the shared generated-app auth layout for setup,
sign-in, password reset, MFA, and email verification. Required account and
workspace details remain first; the workspace language preference is optional.
Authentication mutations retain ordinary CSRF-protected forms and reset
requests retain generic responses. MFA keeps passkey enhancement plus TOTP and
single-use recovery form fallbacks. `auth_journeys_test.go` checks setup,
localized MFA retry and fallback states; `handler_test.go` checks safe passkey
JSON and verification success without token reflection or caching. Verified
2026-11-17: the focused web tests passed 8 tests, `make templ-check` passed, and
the isolated Playwright run passed all 12 email verification and 8 password
reset cases.

Issue #444 migrates `/security` factor and linked-identity rendering to
`security.templ` and the shared DaisyUI/account components. Route tests cover
CSRF-protected factor forms, inaccessible enrollment secrets on the idle page,
focused reauthentication failure feedback, and the ordinary provider-link form.

## Account security form evidence

Issue #443 moves `/account` to `account.templ` and generated `account_templ.go`.
Browser route tests cover password mismatch, invalid current password, successful
password change with session revocation, required deletion confirmation,
owner-transfer refusal, and successful deletion. Password fields remain outside
draft persistence. Email verification request and delivery outcomes retain
ordinary CSRF-protected forms. Verified 2026-11-17 with `make test`,
`make templ-check`, and `make fmt-check`.

## Item short ID evidence

Verified 2026-11-11 with Go 1.27.4. Issue #190 moves item keys to integers
(`items.ItemID`, item type code) and exposes only the item short ID on web, REST,
MCP, CLI, webhooks, exports, and audit metadata. Malformed, unknown, foreign, and
workspace short IDs on item routes return the same not-found. `go test -count=1 ./...`
passed on SQLite, with `make lint` and `make test-unit` passing. A fresh PostgreSQL
14.24 container passed `go test -p 1` for the items, sharing, database,
maintenance, identity, portability, and files packages. The migration rewrite is
covered by the workspace exception record, whose item-key scope the user approved
for #190.
## Item event short ID evidence

Issue #244 moves `item_events` keys to integers under the `item_event` type
code. Change-feed cursors and the `item.changed` event `id` are short IDs.
A malformed cursor, including one stored before this change, returns
`ErrInvalidChangeID` from the service and `resync` on the stream. Retention is
unchanged. The exception is
[2026-11-12-item-event-short-id-migration-rewrite](decisions/exceptions/2026-11-12-item-event-short-id-migration-rewrite.md),
pending owner approval.

Evidence: service cursor and JSON tests in `internal/items`, the SSE malformed
cursor test in `internal/web`, the never-reused key test in
`internal/platform/database`, and the maintenance retention test in
`internal/platform/maintenance`.

## Member, invitation, and API token short ID evidence

Verified 2026-11-11. Issue #193 gives members, invitations, and API tokens integer
keys and exposes only their ADR 0009 short IDs. Token secrets remain random values and
are never encoded. Malformed, unknown, and foreign IDs return the same `not-found`
problem, covered by `TestIdentityRecordPathsAreShortIDOnlyAndUniform` on SQLite.
Checks run: `make test` (all packages, SQLite) and `make lint` passed; `go generate
./api` produced no drift; `make test-postgres` passed for `internal/identity` and
`internal/files` against PostgreSQL 14.24. `internal/httpapi` and the PostgreSQL
retention test were not run on PostgreSQL. The migration rewrite is covered by the
[recorded exception](decisions/exceptions/2026-11-11-workspace-short-id-migration-rewrite.md).

## File short ID evidence

Verified 2026-11-11 with Go 1.27.4. Issue #191 moves file keys to integers and
exposes only the ADR 0009 short ID under the `file` type code. Forward migration
000020 rebuilds `files`, preserving rows and object keys.

- Passed: `make verify` (lint, test, race, deps, workflow, generate drift, build,
  template smoke); `make secret-check`; `make vuln-check` (no affecting
  vulnerabilities); `make test-postgres` (identity and files packages on
  PostgreSQL 14.24); `make docker-smoke`. The smoke script's readiness loop
  absorbed one transient connection reset while the container started.
- `internal/bootstrap` `TestAgentSurfacesLiveSmoke` hit a 5-second login client
  timeout under full parallel `-race` load on 2026-11-11 and again on 2026-11-12.
  It passes alone under `-race` and in a rerun of `make race`; the test is not
  changed by this issue.
- Covered: codec and JSON round trips, REST probes for integer, malformed,
  hex, unknown, and other-workspace file IDs returning identical not-found
  bodies, a SQLite migration test for row preservation and no key reuse, and
  the presign-release test that fails if keys are reused.
- Gap: web file routes have no short-ID probe test of their own; REST probes
  cover the shared service path.

## Webhook and job short ID evidence

Issue #192 moves webhook, webhook delivery, webhook event, and background job
keys to integers. Webhook and delivery IDs, and the event ID in `eventId` and
`Webhook-Id`, are short IDs with their own type codes. Job keys stay internal:
no job route or response exists, so `id.TypeJob` is registered for a future
surface and nothing exposes it. The exception is
[2026-11-11-webhook-job-short-id-migration-rewrite](decisions/exceptions/2026-11-11-webhook-job-short-id-migration-rewrite.md),
accepted by the repository owner on 2026-11-12.

Surface coverage: REST and web routes accept only short IDs; malformed, integer,
or unknown IDs return the same not-found as a missing webhook. There is no
webhook MCP or CLI surface, so none is changed. Retention prunes
`webhook_events` once no delivery references them.

Known limitation, accepted with ADR 0009: the codec does not reject a short ID
issued under another type code. About 3% of webhook short IDs also decode under
the delivery alphabet. Routes rely on the typed parser and the not-found rule.
Revisit the codec if a route ever accepts more than one type.

Verification, run 2026-11-11 with Go 1.27.4 on SQLite: `make lint` passed;
`make test` passed (exit 0); `go test -race` on the job, webhook, and item
packages passed; `make generate-check` produced no drift. PostgreSQL: the full
`go test -p 1 ./...` suite was run against a fresh pinned PostgreSQL 14.24
container; its result is recorded in the change's pull request description.

## MFA passkey short ID evidence

Issue #246 gives `mfa_passkeys` an integer key under the `mfa_passkey` type code.
Passkey IDs in list and removal responses are short IDs; `mfa_passkeys.credential_id`
stays text. MFA challenges and recovery codes keep their text keys and are bearer
secrets matched by hash only. Migration 000015 is rewritten in place under the
[recorded exception](decisions/exceptions/2026-11-12-mfa-short-id-migration-rewrite.md).

- Covered: `TestPasskeyRemovalDecodesIDsStrictly` (SQLite) shows that a malformed
  passkey ID gets the same error as a missing passkey. `TestSQLiteMFATablesUseIntegerPrimaryKeys`
  and `TestPostgresMFATablesUseIntegerPrimaryKeys` check the `mfa_passkeys` key type
  and that `credential_id` stays text.
- Checks run 2026-11-12 with Go 1.27.4: `make test` and `make lint` passed on SQLite;
  `go generate ./api` produced no drift; the focused packages (`internal/identity`,
  `internal/files`, `internal/platform/database`, `internal/platform/maintenance`,
  `internal/platform/id`, `internal/web`) passed against a fresh PostgreSQL 14.24 container.
- Not covered: a real WebAuthn enrollment and passkey list, since the test harness
  has no WebAuthn ceremony; the passkey REST routes have no short-ID probe test of
  their own beyond the service-level check.
- Known limitation: none new for passkeys. PostgreSQL identities never reuse keys.

## Audit event short ID evidence

Issue #248 gives `audit_events` an integer key under the `audit_event` type code.
Audit event IDs in workspace exports are short IDs. Sessions keep their text key
until the owner decides whether session IDs are element identifiers. The exception is
[2026-11-12-audit-event-short-id-migration-rewrite](decisions/exceptions/2026-11-12-audit-event-short-id-migration-rewrite.md).
Evidence: `TestSQLiteAuditTablesUseIntegerPrimaryKeys`, `TestSQLiteAuditKeysAreNeverReused`,
`TestPostgresAuditTablesUseIntegerPrimaryKeys`, and the portability and maintenance tests.
Checks run 2026-11-12 with Go 1.27.4: `make test` and `make lint` passed on SQLite;
`go generate ./api` produced no drift; the focused packages (`internal/identity`,
`internal/files`, `internal/platform/database`, `internal/platform/maintenance`,
`internal/platform/id`, `internal/web`, `internal/portability`) passed against a fresh
PostgreSQL 14.24 container.

## users short ID evidence

Ticket #249 gives `users` an integer key under the `user` type code. Every
column that references `users(id)` (21 columns) is `BIGINT`, and user IDs in
REST, MCP, web, audit, and portability output are short IDs. `external_identities.id`,
`sessions.id`, `oidc_transactions.state_hash`, and the MFA challenge and recovery-code keys
keep text keys. Migrations 000002, 000003, 000005, 000007, 000012, 000013, 000014, 000015,
000016, and 000020 are rewritten in place under the
[recorded exception](decisions/exceptions/2026-11-13-users-short-id-migration-rewrite.md),
which is `accepted`.

- Covered: `TestSQLiteUserKeysUseIntegerReferences` and
  `TestPostgresUserKeysUseIntegerReferences` check `users.id` and the 21 referencing
  columns are integer typed, and that `external_identities.id`, `sessions.id`,
  `oidc_transactions.state_hash`, `mfa_challenges.session_id`, and
  `oauth_grants.session_id` stay text. A missing table fails the test unless it is
  the Files profile table that generated apps without that profile remove, and no
  embedded migration creates it. `TestUserIDJSONEmitsShortIDNotIntegerKey`
  and `TestParseUserIDRejectsMalformedAndZero` check that the typed key emits and
  accepts only short IDs. Login, OIDC, invitation, token, and SCIM flows run on
  their existing identity tests with the new IDs.
- Known limitation: the SCIM backfill in migrations 000012 and 000013 casts the user
  key to text for `scim_users.external_id`. It runs only over existing `workspace_members`,
  which is empty on a fresh database, so it has no effect on the reset path.
- Superseded: audit actor, audit target, idempotency principal, and job dedup
  columns are typed integer keys as recorded in
  [audit and idempotency short ID evidence](#audit-and-idempotency-short-id-evidence).
- Ticket #250 contract surfaces are covered in
  [SCIM and MCP user ID evidence](#scim-and-mcp-user-id-evidence). Field names are unchanged.

## SCIM and MCP user ID evidence

Ticket #250 makes the SCIM resource `id` and group `members.value` the user short
ID. The IdP `externalId` keeps its value and its filter; when omitted it defaults to
the short ID, as #249 set it, so an IdP that never sends `externalId` sees the same
value in `externalId` and `id`. Covered by `TestSCIMResourceIDsAreShortUserIDs`
(lookup and group add by short ID only, not-found for external, malformed, zero,
and unmapped IDs, `scim.user.created` audit target) and `TestSCIMRESTContract`
(HTTP path, Location, and external-ID not-found). The MCP `createdByUserId` schema
declares the short-ID pattern, recorded in `internal/mcpserver/testdata/tools-v1.json`.
The idempotency principal pin (`TestIdempotencyPrincipalUsesShortUserID`) is superseded by
`TestIdempotencyPrincipalIsTypedKey` in the audit and idempotency evidence.
OpenAPI describes the SCIM path parameter and member value; `go generate ./api`
shows no drift. Breaking changes for SCIM IdPs and MCP clients are in the PR body.

## audit and idempotency short ID evidence

Ticket #195 removes the last text entity IDs from audit and idempotency storage.
`audit_events.actor_user_id` is a `BIGINT` user key. `audit_events.target_id` is
replaced by `target_type` (the registry type code) and `target_key BIGINT`.
`idempotency_records.principal_id` is replaced by `principal_kind` and
`principal_key BIGINT`. `background_jobs.deduplication_key` is `BIGINT`, the webhook
delivery key. Migrations 000002, 000003, 000010, and 000013 are rewritten in place under
the [recorded exception](decisions/exceptions/2026-11-13-audit-idempotency-job-key-rewrite.md),
which is `accepted`.

- Covered: `TestSQLiteShortIDKeyColumns` and `TestPostgresShortIDKeyColumns` check
  the typed columns and that `audit_events.target_id` and `idempotency_records.principal_id`
  no longer exist. `TestAuditTargetStoresTypedKeyOrNull` checks that a keyed target
  stores type and key and a key-less target stores `NULL`.
  `TestListAuditHistoryRejectsUnparseableActorRow` checks that a non-positive stored
  actor key is rejected. `TestIdempotencyPrincipalIsTypedKey` checks that a session
  and an API-token principal use separate scopes. `TestEnqueueRejectsNonPositiveDedupKey`
  checks job keys. `TestSQLiteWorkspaceExport` and `TestPostgresWorkspaceExport` check
  that an exported item audit event carries the item's short ID and the actor's
  short user ID.
- Design points for owner review: `principal_kind` also holds `scim_token` and
  `oauth_grant` (not only `user` and `token`), and an MCP OAuth actor's idempotency
  scope moves from the access-token row to the grant. Audit targets with no integer
  key (`session`, `mfa_challenge`, `external_identity`, `notification_preference`,
  `scim_group`) store `NULL` and no longer identify the specific object. Both are
  recorded in the exception.
- Checks run with Go 1.27.4 at `97f5b50`: `make test` and `make lint` passed on SQLite;
  `go generate ./api` produced no drift on two runs; `go test -p 1 -count=1 ./...` with
  `POSTGRES_TEST_URL` passed against a fresh `postgres:14.24-alpine3.24` container.

## Shared navigation migration evidence (#434, 2026-11-16)

Shared navigation renders through templ, with an escaped bridge for remaining
`html/template` pages. Focused bridge and route tests cover active state, role
visibility, translated script messages, and logout CSRF escaping.

- Passed: 15 focused web tests, `go vet ./internal/web`, `make templ-check`,
  and `git diff --check`.
- Passed: `./scripts/playwright-smoke.sh ui.spec.mjs` (10 checks), including
  workspace selection and home snapshots on desktop and mobile in light and
  dark themes, the Items journey, and Spanish workspace creation without JavaScript.
- Passed: `npm run test:a11y` against an isolated server in English and Spanish;
  zero axe violations on covered routes, with keyboard, focus, 320px reflow,
  reduced-motion, and accessibility-tree checks.
- Not run: full test and verification suites, deferred while PR checks run.

## Account and public journey evidence (#453, 2026-11-17)

- Passed: `./scripts/playwright-smoke.sh --grep 'account and security pages|profile preference persists|profile saves and account deletion|session can be revoked'` (7 checks). Account, language, security, and session routes render across English/Spanish, desktop/mobile, and light/dark themes; profile persistence, ordinary-form deletion safeguards, and active-session revocation pass.
- Existing coverage: `scripts/accessibility.mjs` audits setup, sign-in, MFA, verification, recovery, account, language, security, sessions, public share, and MCP consent. `scripts/playwright/password-reset.spec.mjs` and `scripts/playwright/invitations.spec.mjs` cover password recovery and invitation acceptance.
- Gaps: browser screenshots and an expired-session browser fixture remain uncovered; automated accessibility checks do not replace assistive-technology review.

## Account browser follow-up evidence (#453, 2026-11-17)

- Passed: `PLAYWRIGHT_PORT=58395 ./scripts/playwright-smoke.sh account.spec.mjs --grep 'session can be revoked|expired session'` (2/2). An expired fixture session is rejected on its next request and shown without a revoke action; an active session remains revocable.
- Added: `scripts/playwright/account.spec.mjs-snapshots/` records account page baselines for desktop/mobile and light/dark themes. The full account suite must pass before these baselines count as verified.
- Limitation: automated accessibility checks do not replace assistive-technology review.

## Files evidence

Files metadata migration and workspace authorization, local filesystem uploads
and streams, private S3-compatible presigned GET downloads and streamed S3 uploads,
configurable 16 MiB object and 1 GiB workspace limits, safe filename and content
allowlists, browser ordinary-form fallback, REST/OpenAPI behavior, and generated
profile pruning are covered by focused SQLite tests and generated-app checks.
The browser list also uses saved per-member column visibility, typed filters,
stable sorting, keyset pagination, and the shared responsive table/card view;
`TestFilesListUsesSharedPresentationAndSavedColumns` covers the list state and
cursor continuation in `internal/web/files_test.go`.
reconciliation, scanning, background jobs, and PostgreSQL/Docker execution
remain follow-up verification or deferred behavior for this slice.

Browser uploads use one multipart request to the files route. Evidence:
`TestFilesMultipartUploadStoresFileAndRedirects`, `TestFilesUploadStatusMessages`,
`TestFilesMultipartUploadDeniesViewer`, and `TestFilesInitiateAndCompleteRoutesReturnNotFound`
in `internal/web/files_test.go`; `FileUploader` lock and status transitions in
`internal/web/assets/app.test.mjs`. Verified with `go test ./internal/web/...`
and `node --test internal/web/assets/*.test.mjs`. Not run against a real S3
bucket for this path.

## HTMX 4 upgrade evidence

Verified 2026-11-10 with Go 1.27.0, HTMX 4.0.1, axe-core, and Chrome:

- vendored `htmx.min.js` is the npm `htmx.org@4.0.1` `dist` build; the
  upgrade-check scanner reports no remaining old events in application code
- `app.js` listens for `htmx:after:swap` and restores HTMX 2.x non-swap for
  `204`, `304`, `4xx`, and `5xx` so error pages never replace `#item-list`
- `go test ./...` passes, including the HTMX create, validation, and delete
  handler tests
- `make accessibility-smoke` passes with zero axe violations on login,
  workspaces, workspace, general settings, items, and item-validation, plus the
  keyboard, focus, reflow, reduced-motion, and accessibility-tree checks
- `make webmcp-smoke` passes the ordinary-fallback check on Chrome; WebMCP tool
  assertions were skipped because the browser exposes no `document.modelContext`
- not yet covered: a browser-level check that a 4xx response leaves the list
  unchanged under the restored non-swap config

## MCP MRTR approval evidence

Verified 2026-11-10 with Go 1.27.3 and go-sdk v1.8.0:

- `internal/mcpserver/approval_test.go` covers update and delete confirmation:
  `input_required` with a `human-approval` elicitation and request-state binding,
  no mutation before an accepted `confirm: true`, a retry on a second connection
  (stateless), decline, forged request state, changed arguments, a header-only
  call that does not approve, and a pre-MRTR protocol call with the old header
  that cannot update the item
- mutation check: removing the request-state comparison fails the forged-state
  assertion
- `go test ./...`, `go vet ./...`, `go generate ./api` with no drift, and
  `make verify` (including race) passed on the final code
- not run: `make docker-smoke`, `make secret-check`, `make vuln-check`,
  `make fuzz-smoke`, PostgreSQL checks (no data change), and the browser and
  WebMCP smoke targets (no browser surface change)
## MCP receiving middleware evidence

Verified 2026-11-10 with Go 1.27.0 on SQLite:

- receiving middleware audits, times, and recovers panics for every method the
  SDK dispatches to the server: tools, `tools/list`, and `prompts/list` are
  covered by `TestMCPReceivingMiddlewareAuditsEveryKnownMethod`; a panicking
  tool returns `internal failure`, leaks no panic value to logs, and is audited
  by `TestMCPReceivingMiddlewareRecoversPanicsWithoutLeakingValues`
- unregistered tool names are audited as `tools/call` without the client name
  by the same test; unknown method names are rejected by the SDK before
  middleware and write no audit record, pinned by
  `TestMCPUnknownMethodIsRejectedBeforeMiddleware`
- the existing tool-audit contract test still asserts ten `mcp.tool.called`
  events, `tool_version`, request ID, and no bearer or title leakage
- `go test ./...`, `go vet ./...`, `go generate ./api` with no drift,
  `make deps-check`, `make workflow-check`, `make generate-check`,
  `make build`, and `make template-smoke` pass
- `go test -race ./internal/mcpserver/` passes; the full `make race` run hit a
  timeout in `internal/bootstrap` `TestAgentSurfacesLiveSmoke` (login returned
  504 after 35s under full-suite load) and that test passes alone with race
- not run: PostgreSQL compatibility (`POSTGRES_TEST_URL` unset), `make
  docker-smoke`, `make secret-check`, `make vuln-check`, `make fuzz-smoke`
- not covered: an audit-write-failure test (the `internal failure` branch), and
  notifications, which are outside receiving middleware

## MCP resources evidence

Verified 2026-11-10 with Go 1.27.3 on SQLite:

- [resource tests](../internal/mcpserver/resources_test.go) cover the template listing, own-workspace read with
  `application/json` content, unknown-item and item-in-other-workspace
  not-found, cross-workspace `access is denied` without item data, malformed
  URI rejection, write-only token rejection at connect, read-only token reads,
  and no `Mcp-Session-Id` issued on resource reads
- `make verify` exits 0: lint, `go test ./...`, `go vet ./...`, `go test -race
  ./...`, dependency and workflow checks, generated OpenAPI no-drift, command
  builds, and generated-app smoke
- not run: PostgreSQL compatibility (no data change), `make docker-smoke`,
  `make secret-check`, `make vuln-check`, `make fuzz-smoke`
- omitted surfaces, by reason: REST/OpenAPI and CLI need no resource form
  because items already have REST and CLI item APIs; WebMCP and browser are
  not agent-automation surfaces for read-only resources; realtime delivery and
  resource subscriptions are not needed because no client requires change
  notifications

## MCP prompts evidence

Verified 2026-11-10 with Go 1.27.0 and go-sdk v1.8.0 on SQLite:

- `items_summarize_v1` and `items_triage_v1` are listed and fetched through the
  stateless `prompts/list` and `prompts/get` methods; no `Mcp-Session-Id` is
  issued
- arguments are validated strictly (undeclared, missing, malformed, or
  unsupported values return `-32602` with `parameters are invalid`); unknown
  prompt names are rejected by the SDK
- prompts require `resources:read`: a write-only token receives HTTP `403`, a
  read-only token gets results, and a foreign workspace returns `access is denied`
- item titles are JSON-encoded inside a `<workspace_items>` block, and the item
  table is byte-identical before and after prompt calls (no mutation)
- every prompt call writes an `mcp.prompt.called` audit record without the bearer
  token or item titles
- `go test -count=1 -race ./internal/mcpserver/` passes, covering the prompt tests
  in `internal/mcpserver/prompts_test.go`
- `go test ./...`, `go vet ./...`, `make race`, `make deps-check`,
  `make workflow-check`, `make generate-check` (no OpenAPI drift), `make build`,
  and `make template-smoke` each exited 0
- `make verify` as one invocation first stopped at `make race` because
  `TestAgentSurfacesLiveSmoke` (`internal/bootstrap`) timed out on `/login` while
  the host load average was about 335; the test passed when run alone under
  `-race`, and the full `make race` passed on rerun
- not run: `make docker-smoke`, `make vuln-check`, `make secret-check`, and
  `make fuzz-smoke`; prompt behavior is not covered by a browser or PostgreSQL run

## MCP cacheable tool catalog evidence

Verified 2026-11-10 with Go 1.27.3, SQLite, and the vendored `go-sdk` v1.8.0:

- `tools/list` carries `ttlMs: 300000` and `cacheScope: "public"`; `ping` gains
  no cache fields; the catalog still lists the five `items_*_v1` tools; no
  `Mcp-Session-Id` is issued ([test](../internal/mcpserver/server_test.go))
- `go test ./...`, `go vet ./...`, `go test -race ./internal/mcpserver`, and
  `go generate ./api` with no generated drift pass
- `make verify` passes, including lint, actionlint, dependency and secret checks,
  generate check, build, and the generated-app template smoke test
- not covered: no resources or prompts exist, so their list hints are untested;
  no live-client cache behavior was exercised

## MCP typed-tool evidence

Verified 2026-11-10 with Go 1.27.3 on the rebased branch commit, after
rebasing onto `main` at `f5635f3`:

- `make verify` exited 0: formatting, `go vet ./...`, `go test ./...`,
  `go test -race ./...`, `go mod verify`, workflow checks, reproducible
  `go generate ./api` with no drift, command builds, and generated-application
  tests for the Core, Identity, and Agent profiles
- focused MCP tests cover the golden `tools/list` contract in
  `internal/mcpserver/testdata/tools-v1.json`, the SDK-applied published
  defaults, and rejected-argument audits without workspace attribution
- not run: `make docker-smoke`, `make secret-check`, `make vuln-check`,
  `make fuzz-smoke`, and the PostgreSQL suite; this change alters no SQL or
  migrations, and the MCP tools still reach data only through the item service
- known ceiling: output-validation failures after a handler returns are audited
  as success before the SDK rejects the output; outputs come from typed DTOs, so
  this is treated as a programming-error edge rather than a reachable input path

## MCP completion evidence

Verified 2026-11-10 with Go 1.27.3 and the go-sdk v1.8.0 stateless endpoint:

- completion and prompt tests cover the advertised `prompts` and `completions`
  capabilities, stateless responses without `Mcp-Session-Id`, read-scope
  enforcement (a write-only token is denied), read-only token access, status
  and workspace-scoped item ID completion, empty results for unauthorized,
  malformed, or missing workspaces, a 100-value bound with `hasMore`, and
  prompt authorization through the shared item service
- `go vet ./...`, `go test -race ./internal/mcpserver/`, and `go generate ./api`
  with no generated drift pass
- `go test ./...` passed in every package except `internal/bootstrap`, where
  `TestAgentSurfacesLiveSmoke` timed out once under full-suite load; the same
  test passed on an isolated rerun, so the timeout is recorded as unresolved load
  sensitivity, not as a fix
## MCP built-in authorization server evidence (partial)

Verified 2026-11-10 with Go 1.27.3 on macOS (SQLite only):

- `internal/identity/mcp_oauth_test.go`: client redirect validation, PKCE and
  binding checks, code reuse revokes the grant, refresh rotation with reuse
  detection, expiry, membership removal, session revocation, consent permission
  checks, and revocation
- `internal/mcpserver/oauth_token_test.go`: token issuance and refresh over HTTP,
  OAuth-format errors without echoing secrets, read-only scope enforcement,
  `error="invalid_token"` challenges, and revocation
- `make verify` passed on this tree (gofmt, vet, `go test ./...`,
  `go test -race ./...`, generate-check, build, template-smoke)
- `POST /oauth/register`, RFC 8414 metadata, and the localized consent page
  (`internal/web/mcp_consent_test.go`: sign-in redirect, escaped client name,
  invalid redirect and resource rejection, CSRF, approve and deny, code exchange)
- `internal/identity/mcp_oauth_postgres_test.go` on PostgreSQL 16
- `make accessibility-smoke` audited the public share and consent pages in
  English and Spanish with zero axe violations (2026-11-16)
- not yet implemented: expired-record pruning

## Object storage S3 conformance evidence

Verified 2026-11-11 with Go 1.27.4 on the in-process conformance suite:

- `TestS3ConformanceInProcess` (`internal/files/s3_conformance_test.go`) runs
  the S3 object contract through the production `newS3Store` constructor against
  an in-process S3-compatible endpoint. It covers a put/head/get round trip with
  size and content type, conditional create rejecting overwrite, not-found
  normalized to `ErrObjectNotFound` for head and get, a presigned GET whose
  `X-Amz-Expires` equals the requested TTL, unsigned requests refused so objects
  stay private, and a presigned GET refused after expiry. The presigned PUT
  assertions were removed in #272.
- mutation checks: disabling the expiry check fails the suite at the expired-URL
  assertion; disabling `If-None-Match` fails at the overwrite assertion
- the in-process endpoint does not verify SigV4 signatures. Signature validity
  requires a run against a real endpoint
- `TestS3ConformanceConfiguredEndpoint` runs the same suite when
  `S3_CONFORMANCE_ENDPOINT` and `S3_CONFORMANCE_BUCKET` are set, and skips
  otherwise. Credentials come from the AWS SDK standard chain and are not logged
- no new storage driver; the change adds tests and documentation only
- passed: `make test`, `make lint`, `make doc-links`, `make generate-check`,
  `make secret-check`, and `go test -race ./internal/files/...`
- not run: `make race` for the whole tree, PostgreSQL (`POSTGRES_TEST_URL` unset;
  no data change), `make docker-smoke` (no runtime change), and `make verify`
- Rechecked 2026-11-14 with Go 1.27.4 after #272: `make test`, `make lint`,
  and `verification/tools/run-tlc.sh verification/files FileUpload` passed; the
  in-process suite still checks presigned GET expiry.

Manual R2 run: **not run**. It is gated on R2 credentials and a dedicated test
bucket, neither of which was available to this run. Procedure:
[deployment](deployment.md#cloudflare-r2-object-storage). Record the result as a
new dated section here: date, endpoint kind (no account ID), Go version, and
`go test` outcome.

Not verified against R2 until that run passes: SigV4 signature validity for
presigned GET URLs, the SDK's default request checksum behavior, and conditional
PUT with `If-None-Match`. Browser CORS preflight: **not applicable**. Uploads stream
through the application, so the browser sends no request to the bucket
([ADR 0010](decisions/0010-streamed-s3-uploads.md)). The presigned PUT path that
required bucket CORS was removed in #272.

## Streamed S3 upload evidence

Verified 2026-11-13 with the in-process S3 endpoint ([ADR 0010](decisions/0010-streamed-s3-uploads.md)):

- REST initiate on S3 returns an application `uploadUrl` with no upload headers
  and a `completeUrl`; the content route requires a bearer token; a read-only
  token is refused with `insufficient-scope`
  (`TestS3InitiatedUploadStreamsThroughApplication`, `internal/httpapi/files_test.go`)
- the content write stores bytes and leaves the file pending; `complete` makes it
  ready; a viewer's content write is refused; a failed store write releases its
  quota (`TestStreamedS3UploadStaysPendingUntilComplete`,
  `TestStreamedS3ContentWriteDeniesViewer`, `TestStreamedS3WriteFailureReleasesQuota`,
  `internal/files/s3_streamed_test.go`)
- passed: `make test`, `make lint`, `go test -race ./internal/files/... ./internal/httpapi/...`,
  and `go generate ./api` with no drift on two runs (no OpenAPI text changed)
- blocked: `TestS3ConformanceConfiguredEndpoint` against R2 or S3 needs
  `S3_CONFORMANCE_ENDPOINT` and `S3_CONFORMANCE_BUCKET`, which were not set in this run
- not applicable: `make docker-smoke` (no delivery files changed)

## Workspace mode evidence

Verified 2026-11-11 with Go 1.27.4 on SQLite. Behavior for both modes is
documented in [web](web.md) and
[configuration](configuration.md). Evidence:

- mode parsing: `TestLoadFromWorkspaceModeDefaultsToSingle` and
  `TestLoadFromRejectsInvalidWorkspaceMode` in
  [config tests](../internal/platform/config/config_test.go)
- single mode: `TestWebRootLandsInOldestWorkspaceInSingleMode`,
  `TestWebRootShowsNoMembershipsInSingleMode`, and
  `TestSingleModeHidesWorkspaceListNavigation` in
  [browser mode tests](../internal/web/handler_test.go)
- multi mode: `TestWebRootListsWorkspacesInMultiMode` and
  `TestMultiModeKeepsWorkspaceListNavigation` in
  [browser mode tests](../internal/web/handler_test.go)
- the accessibility and WebMCP smoke scripts run the server with
  `MULTI_WORKSPACE_ENABLED=true` so their workspace-list flows stay covered

- single mode blocks browser workspace creation: the create form is hidden and
  `POST /workspaces` returns `404`. Evidence:
  `TestSingleWorkspaceModeHidesCreationAndReturnsNotFound` in
  [workspace mode tests](../internal/web/workspace_mode_test.go)
- multi mode keeps browser workspace creation: `TestMultiWorkspaceModeKeepsCreation`
  in [workspace mode tests](../internal/web/workspace_mode_test.go)

- passed: `make test`, `make lint`, `make doc-links`, `make secret-check`, and
  `go test -count=1 -run` for the named config and web mode tests
- not run: PostgreSQL (`POSTGRES_TEST_URL` unset; no data change), `make race`
  (no concurrent code changed), `make generate-check` (no API change), and
  `make docker-smoke` (no runtime change)

## Identity key and short-ID documentation evidence

Ticket #196 records the identity key design and aligns the public docs with it.
[ADR 0011](decisions/0011-identity-integer-keys.md) covers integer entity keys,
the ADR 0009 codec with its optional seed and type codes, strict decode, and the
typed audit, idempotency, and job keys. It is `accepted`: the owner approved its
two points (principal kinds, and key-less audit targets) on 2026-11-13.
The ADR is linked from the [documentation router](README.md). The glossary defines
short ID, type code, and integer key. Architecture, API, MCP, CLI, and the upgrade
note describe short IDs and typed keys; `id.New()` values are described only as
credentials and protocol state.

- Covered: the ADR's statements match the code. `internal/platform/id` holds
  the 19-code registry and the empty-seed production codecs, and
  `TestTypeRegistryCodesAreUniqueAndNonEmpty`, `TestCodecRoundTripsEachTypeCode`,
  `TestDifferentSeedChangesEncodingForSameTypeAndInteger`, and
  `TestEntityKeysRejectMalformedShortIDs` cover the registry, codec, seed, and
  strict decode. The cross-type limit is the known limitation in the
  [webhook evidence](#webhook-and-job-short-id-evidence).
- Covered: `make doc-links` passed, and every relative link added in this change
  resolves to an existing file. Anchors added in this change point to existing
  headings.
- Stale hex check: `git grep -n -i hex -- docs` finds no current-tense claim that
  an entity ID is hex. Remaining hits are credential encodings in configuration,
  the `id.New()` description in architecture, the retired-format note in the CLI
  and upgrade docs, and the file-route probe in the file evidence. Dated evidence
  and exception records are unchanged.
- Not applicable: no code, migration, OpenAPI, or runtime change. `make generate-check`
  passed with no drift, and `make docker-smoke` was not run (no runtime change).
- Passed: `make test`, `make lint`, `make generate-check`, and `make doc-links`
  on branch `docs-short-id-196`.
- Owner approval: ADR 0011 and its two points, on 2026-11-13. The audit and idempotency
  exception was accepted by the owner and security owner on 2026-11-13 (PR #288); this
  change does not touch it.

## Workspace home and styling foundation evidence (2026-11-15)

Ticket #375 moves the workspace home to generated templ Go, adds a first-task
link and responsive tool cards, and uses shared light/dark/system theme state.
The app name is generated through `common.product_name`; brand color, font,
radius, and spacing tokens live in `internal/web/assets/app.css`. Tailwind CSS
4.3.5 and daisyUI 5.8.1 compile the embedded button stylesheet; templ 0.3.1020
generates and commits Go output.

- Passed: `make lint`, `make test-unit`, `go test ./internal/web ./internal/i18n -count=1`,
  `make templ-check css-check`, `make accessibility-smoke` (English and Spanish;
  0 axe violations; keyboard, focus, 320px reflow, reduced motion, and
  accessibility tree checks passed), and `make template-smoke`.
- Not run: full `make verify`; the ticket's focused, accessibility, and
  generated-app checks passed.

## Items journey presentation evidence (2026-11-15)

Ticket #376 aligns the Items page with the shared brand, adds a clear purpose
heading and find/sort label, and improves responsive create and edit layouts.
Active and complete states remain readable as text labels; query, form, edit,
autosave, conflict, and destructive-action behavior stays on the existing
server-rendered path.

- Passed: focused Items, form-markup, and create-model tests; `make ux-smoke`
  (42 checks); and `make accessibility-smoke` in English and Spanish (0 axe
  violations; keyboard, focus, 320px reflow, reduced motion, and accessibility
  tree checks passed).
- Not run: full `make test` and `make verify`; no domain or persistence behavior
  changed.

## Client interface evidence (2026-11-15)

Ticket #377 applies the shared branded workspace shell to client lists, details,
and custom-field management. Client lists keep shareable query state and
responsive table/card rendering. Permanent client and field deletion effects
are visible next to their controls, including without JavaScript.

- Passed: `go test ./internal/web -count=1`, `make lint`, and
  `make accessibility-smoke` in English and Spanish (zero axe violations;
  keyboard, focus, 320px reflow, reduced-motion, and accessibility-tree checks).
- Not run: full `make verify`; this ticket's web, lint, and accessibility checks
  passed.

## Client custom-field templ migration evidence (2026-11-17)

Issue #441 migrates `/workspaces/{workspaceId}/client-fields` to generated
`client_fields.templ` and shared DaisyUI form, alert, and confirmation
components. Field and choice-option lifecycle remains on the shared clients
service. Route checks cover create, archive, restore, permanent delete,
option add/delete, validation, permission denial, escaped labels, and explicit
deletion consequences with ordinary CSRF-protected forms.

- Passed: `go test ./internal/web` (212 tests), `make templ-check`, and
  `make lint`, `make test` (39 JS and 1,070 Go tests), and the focused
  Playwright custom-field lifecycle journey.
- Not run: `npm run typecheck` and `npm run test`; this repository defines no
  such npm scripts. The full browser accessibility suite was not run.

## File management progress and localization evidence (2026-11-15)

Ticket #378 adds determinate byte progress to the existing single-request
multipart upload, while retaining the ordinary form fallback. The native
progress element tracks computable upload events; the polite status announces
quarter milestones. Phase messages and safe server errors follow the selected
English or Spanish locale. File filters, sorting, pagination, responsive table
and card views, and workspace-authorized download routes remain in use.

- Passed: `go test ./internal/web -count=1` (160 tests), `make test-unit` (25
  tests), `make lint`, `node --check internal/web/assets/app.js`, and
  `node --check scripts/accessibility.mjs`.
- Passed: `make accessibility-smoke` in English and Spanish (zero axe
  violations; files page, named progress control, keyboard/focus, and 320px
  reflow checks passed).
- Not run: full `make test` and `make verify`.

## Member and invitation UX evidence (2026-11-15)

Ticket #379 adds role permission guidance and visible ownership-transfer
consequences to member administration. Invitation pages explain that acceptance
requires the invited email, give sign-in and sign-out paths for existing or
wrong accounts, and tell users how to recover from unavailable links. Failed
acceptance keeps one generic message; shared identity policy remains unchanged.

- Passed: `go test ./internal/web -count=1` (161 tests), `make lint`, and
  `make accessibility-smoke` in English and Spanish (members and invitations:
  zero axe violations; keyboard, focus, 320px reflow, reduced motion, and
  accessibility tree checks passed).
- Not run: full `make verify`; no authentication policy, schema, or migration
  changed. `CHANGELOG.md` is aggregated at PR merge time.

## Workspace settings and integrations evidence (2026-11-15)

Ticket #380 groups general and email setup, access and integrations, and data
and activity in the workspace settings navigation. Settings pages use the
generated product name and shared responsive panels. API token, webhook, and
SCIM credential notices explain one-time display and replacement or rotation.
Authorization checks, secret storage/display behavior, no-store responses, and
WebMCP exclusions remain in the existing handlers.

- Passed: focused settings and secret-lifecycle tests, `go test ./internal/web ./internal/i18n -count=1` (162 tests), `make lint`, `make ux-smoke` (42 checks), and `make accessibility-smoke` in English and Spanish (zero axe violations; all covered settings and browser journeys passed).
- The isolated accessibility smoke server uses higher request limits so its complete route audit does not hit production defaults.
- Not run: full `make test` and `make verify`.

## Invitation return and acceptance evidence (2026-11-17)

Ticket #421 stores invitation return state in the database as opaque,
browser-bound hashes. The short-lived handoff stores the invitation ID, not its
raw secret. Return targets use one fixed route; acceptance checks the invited
account and consumes the handoff atomically with membership creation. Expiry,
revocation, resend, replay, pruning, request-log redaction, CSRF, no-store, and
no-JavaScript form behavior have regression coverage.

- Passed: `make test` (34 browser unit tests and all Go packages), `make lint`,
  `make test-postgres` (identity and file packages),
  `go test -race ./internal/identity ./internal/web -count=1` (337 tests),
  `make templ-check`, `make css-check`, and the isolated invitation Playwright
  journey (no JavaScript).
- Passed: invitation lifecycle model check (61 distinct states, no invariant
  errors).
- Not run: full `make verify`; `CHANGELOG.md` is aggregated at PR merge time.

## Import preview evidence (2026-11-15)

Ticket #381 adds a read-only preview for bounded JSON and strict CSV item
imports. The preview validates every record, reports the record count, samples
at most five records, and returns at most ten actionable row errors. It writes
no items. Confirmation requires the same file digest, then the existing
portability transaction rechecks workspace permission and item quota before
creating every row atomically; duplicate titles stay distinct. Both steps use
ordinary forms and need no JavaScript.

- Passed: `go test ./internal/portability ./internal/web ./internal/i18n -count=1`
  (169 tests), `make test` (33 browser unit tests and all Go packages),
  `make lint`, and English/Spanish
  `make accessibility-smoke` (zero axe violations; keyboard, focus, 320px
  reflow, reduced motion, and accessibility-tree checks). Focused tests cover
  no-write preview, row-error bounds, duplicate titles, changed-file rejection,
  and stateless preview/confirmation across two handler instances.
- After stacking #376–#380, `make ux-smoke` passed all 42 checks and
  `make accessibility-smoke` passed in English and Spanish across all covered
  journeys, including import preview; zero axe violations.
- Not run: full `make verify`.

## Account and security UI evidence (#382)

Ticket #382 refreshes the account, language, sessions, security, and password
recovery pages on the shared responsive shell. Account deletion now requires
both the existing password and an explicit confirmation field; the handler
enforces the confirmation and retains the existing owner-transfer restriction.
Password reset errors retain the recovery form and valid CSRF state. EN/ES
strings cover the changed screens.

- Passed: `go test ./internal/web ./internal/i18n -count=1`,
  `make lint`, `git diff --check`, and `node --check scripts/accessibility.mjs`.
- Passed: `make accessibility-smoke` audited `/account`, `/settings/language`,
  `/sessions`, `/security`, and `/password-reset` in English and Spanish with
  zero axe violations and no 320px overflow.
- After stacking #376–#381, `make ux-smoke` passed all 42 checks and full
  `make accessibility-smoke` passed in English and Spanish across all covered
  journeys, including account and security; zero axe violations.
- Not run: full `make verify`; no authentication policy, schema, or API changes.

## Account profile, preferences, language, and appearance evidence (#442)

The `/account` and `/settings/language` templ pages render with the saved
English or Spanish locale and light, dark, or system theme. Existing handler
checks cover successful profile and notification preference saves, invalid
profile and language submissions without overwriting saved values, and
automatic language fallback. A focused rendering matrix covers both routes in
all six locale/theme combinations.

- Passed: `go test ./internal/web ./internal/i18n -count=1`,
  `npm run test:unit`, `make templ-check`, and `make css-check`.
- Blocked: `make lint` and `make test` stop at the existing broken relative
  Markdown link in `docs/htmx-review-438.md`.
- Not run: `npm run typecheck` and `npm run test`; `package.json` defines
  neither script.

## Session list and revocation UI evidence (#445, 2026-11-16)

Ticket #445 migrates `/sessions` to the generated templ account layout. The
page labels the current session and shows active, expired, and revoked status.
CSRF-protected ordinary forms revoke any active session, including the current
session; the latter redirects to sign-in on the next request. EN/ES cover the
status, feedback, and empty states. The page remains no-store and keeps its
WebMCP page marker and script.

- Passed: session rendering and lifecycle tests, including current-session
  revoke, cross-user denial, CSRF rejection, expired state, Spanish empty state,
  and ordinary form behavior; `make test`, `make lint`, `make templ-check`, and
  `make css-check`. English and Spanish `make accessibility-smoke` reported
  zero axe violations, including sessions, keyboard/focus, and 320px reflow.
  `make ux-smoke` passed 42 checks; `make browser-smoke` passed 14 Playwright
  tests. See [session evidence](reviews/sessions-445-evidence.md).
- Not run: full `make verify`, race suite, security gates, and PostgreSQL tests.

## Items list and editing UI evidence (#413, 2026-11-16)

Ticket #413 migrates the workspace Items page and list fragment to generated
templ views. The route keeps existing workspace authorization, CSRF checks,
localized profile theme, ordinary form submissions, list preferences, sharing,
and responsive table treatment. Route-only JavaScript preserves dirty item
rows and standalone drafts across list swaps, applies successful row versions,
and retains submitted values on edit validation errors. Viewers see a read-only
list; create, edit, and delete writes remain forbidden.

- Passed: focused Items permission, list/preferences, and locale/theme Go tests;
  Items JavaScript regressions; `make verify` on `a605f7c`; and the normal
  generated-template, CSS, race, build, and repository checks.
- Passed: final ordered Playwright suite (30 passed, 2 notification tests
  skipped). It covers dirty row and draft preservation, successful autosave
  version updates before delete, hx-sync overlap, native query history, Spanish
  axe, ordinary no-JavaScript forms, and explicit light/dark/system themes.
  Desktop/mobile light/dark screenshot baselines were visually reviewed.
- Passed: `make accessibility-smoke` in English and Spanish with zero Items
  violations and keyboard/focus/reflow checks; `make ux-smoke` (42 checks);
  `make webmcp-smoke` fallback; and `make docker-smoke`.
- Passed: final spec and standards reviews found no actionable defects. The
  browser regression reproduces and guards the stale-version case where a dirty
  create draft used to cancel an unrelated successful item row swap.
- CI history: an earlier PR head failed accessibility on an ambiguous Items
  heading locator and CSP-blocked inline axe. Both test causes were fixed and
  the previous pushed head passed all 10 CI checks. The updated head still needs
  its CI run.
- Pending: normal-hook commit/push, updated-head CI, and merge verification.

## Notification center UX evidence (2026-11-15)

Ticket #383 restyles the recipient-scoped notification list, makes unread
counts and read state visible, and updates the count plus a polite status from
recipient-scoped SSE. The refresh link and server-rendered page work without
JavaScript; the notification accessibility runner checks the empty state.

- Passed: focused notification handler and JavaScript tests, `make test-unit`,
  `make lint`, `make ux-smoke` (42 checks), and full `make accessibility-smoke`
  in English and Spanish across all covered browser journeys (zero axe
  violations; notifications passed both locales).
- Not run: full `make verify`.

## Public sharing and MCP consent evidence (#385)

Public share pages identify the product, show only the shared item title and
status, and explain the link's scope. Invalid, expired, and revoked links use
the same localized unavailable response. Share creation and revocation show
their consequences in the ordinary workspace form. MCP consent identifies the
client, requested scopes, and selected workspace, and keeps approval as an
ordinary CSRF-protected form submission. Existing no-store, no-referrer,
noindex, session, workspace, scope, and redirect validation remain in force;
these pages do not enable WebMCP.

- Passed: focused public sharing, MCP consent, and WebMCP surface tests;
  `git diff --check`.
- Passed: focused public sharing, MCP consent, and WebMCP surface tests,
  `go test ./internal/web -count=1` (158 tests), `make lint`, and
  `git diff --check`.
- Not run: full `make test` and `make verify`; no data, API, or runtime contract
  changed.

## UI migration and generated-app evidence (#386, 2026-11-16)

Ticket #386 inventories all browser page and shared-template files in
[the web contract](web.md#rendering-migration-inventory). The workspace home
is generated by templ. The remaining named `html/template` views are explicitly
deferred there by route family; this is a rendering migration boundary, not an
exception to existing escaping, authorization, fallback, localization, or
accessibility requirements.

- `passed`: `make verify` (lint, unit and race suites, dependency and workflow
  checks, generated API and templ/CSS drift checks, build, and generated-app
  template smoke).
- `passed`: `make ux-smoke` (42 checks).
- `passed`: `make browser-smoke` (five Playwright/Chrome journeys, including
  desktop and mobile screenshots for light and dark themes).
- `passed`: `make accessibility-smoke` in English and Spanish; zero axe
  violations on covered routes, including public share and MCP consent, plus
  keyboard, focus, 320px reflow, reduced-motion, and accessibility-tree checks.
- `not applicable`: PostgreSQL and Docker smoke; this change adds no database,
  API, or runtime behavior.
- The first `make verify` attempt stopped at `css-check` because this fresh
  worktree had no installed npm packages. After `npm ci`, the complete rerun
  passed.

## Shared templ components (#411, 2026-11-16)

Workspace selection and home use shared headings, fields, feedback, buttons,
and cards. [Component rendering tests](../internal/web/components_test.go)
also cover the table, confirmation, validation, empty-state, and pagination
interfaces for the existing route patterns. A Chrome component fixture checks
the paired table and cards at mobile and desktop widths in both themes,
including keyboard focus, standalone table visibility, and axe. Those route
migrations remain separate; other unused components still require browser
validation as they are adopted.

- Passed: focused component and workspace tests; templ and CSS drift checks;
  lint; Chrome workspace screenshots and journeys; English and Spanish
  accessibility smoke; generated-app template smoke.
- Passed: [templ drift fixtures](../scripts/test-templ-check.sh), including
  generator failure and added, removed, or changed outputs; the
  [responsive list fixture](../scripts/playwright/components.spec.mjs).
- Deferred: full `make test` and release verification. No data, API, CLI,
  MCP, realtime, or operational behavior changed.

## Member management validation evidence (#418, 2026-11-17)

The Members route uses a responsive shared list with per-user columns, search,
role filters, sorting, and opt-in paging. Separate ordinary CSRF-protected
forms change roles, remove members, and transfer ownership. The server requires
explicit removal and transfer confirmation; shared identity authorization
continues to enforce owner/admin management and owner-only transfer.

- Passed: `make test`, `make lint`, `go test ./internal/web/...` (198 tests),
  and `go test -race ./...` (1,050 tests across 36 packages) during initial
  #418 validation. The race run overlapped another worktree's race run; no
  process was interrupted, and this run exited successfully.
- Passed: latest normal pre-push `make ci-local` on `35d759e`; hosted CI
  [43997171044](https://github.com/dynamiscode/dynamis-code-apps-template/actions/runs/43997171044)
  passed `make verify`, PostgreSQL `go test -race ./...`, browser and
  accessibility smoke, WebMCP fallback, container, security, and fuzz checks.
  Source security run
  [43997171088](https://github.com/dynamiscode/dynamis-code-apps-template/actions/runs/43997171088)
  passed.
- Passed: full `PLAYWRIGHT_PORT=58125 ./scripts/playwright-smoke.sh` (20/20),
  including role, removal, transfer, permission denial, CSRF, validation,
  keyboard, Spanish, no-JavaScript forms, mobile axe, and screenshot comparisons
  against desktop/mobile light/dark baselines during initial #418 validation.
- Passed: hosted browser smoke compared desktop snapshots and separate Linux
  mobile light/dark snapshots. The Linux baselines match reviewed hosted CI
  actuals; Darwin and desktop snapshot names and the 5% threshold are unchanged.
- Partial: pinned Linux MCR Chrome 157.0.8103.78 member run passed 4/6 tests.
  Desktop and non-screenshot member tests passed; mobile comparisons expected
  390×1268 and received 390×1244. Hosted CI passes those same mobile checks.
- Passed: `A11Y_PORT=58126 make accessibility-smoke` in English and Spanish;
  Members had zero axe violations in both locales.
- Passed: `WEBMCP_PORT=58127 make webmcp-smoke` fallback and member browser
  action coverage. Native WebMCP was unavailable in the installed Chrome, so
  the member browser test invoked the production registered tool through its
  test shim. It verified the tool prepares and focuses the existing form and
  does not submit it; native-browser WebMCP integration remains unverified.
- Passed: `go generate ./internal/web` with zero generated drift.

## UI performance baseline evidence (#439, 2026-11-17)

- Measured one cold and warm workspace-home navigation and cold/warm settings,
  provisioning, webhooks, audit, import, export and delivery navigation on
  commit `1c2ff7f59d106988976daf1370e52463f60d0896` using isolated synthetic
  SQLite fixtures, Chrome 157.0.8103.79, Playwright 1.64.1 and an unthrottled
  loopback server. Full values and boundaries are in
  [UI measurement](ui-measurement.md) and
  [administration measurement](ui-measurement-admin.md).
- Measured selected warm audit-fragment, import-preview and export interactions.
  Export wire bytes are unavailable; CDP reported a transport failure although
  Playwright completed a local download. One cold favicon 404 appeared on each
  measured navigation; no fix is claimed.
- Partial: Items, Clients, Files, members/invitations, account/security,
  sessions/notifications, setup/login/MFA/recovery and public share have
  behavior tests but no performance measurements. State coverage remains
  incomplete for measured routes too. Both docs list unverified states and
  current route-ticket owners.
- No performance budget or optimization was proposed from one observation per
  cache condition. Per-resource attribution, HTTP compression, upload wire
  bytes, SSE, loading durations, and repeated-run variance remain unmeasured.

## UI performance baseline rerun (#439, 2026-11-18)

- Repeated workspace-home and administration measurements on
  `d47ad87eab45db60315310a662e3d57d46a91e4e` with isolated SQLite fixtures.
  Updated values and exact commands are in
  [UI measurement](ui-measurement.md) and
  [administration measurement](ui-measurement-admin.md).
- All cold navigations again reported a favicon 404. Export completed to a
  local file, but transfer bytes remained unavailable. These observations do
  not add per-resource attribution, loading-state timings, or repeated-run
  distributions.
- Route inventory and incomplete state coverage remain as documented above.
  No performance budgets or fixes are proposed from single samples.
- `go test ./...` passed (1070 tests in 37 packages). `npm run typecheck` and
  `npm run test` are unavailable because neither script is defined.

## UI baseline verification follow-up (#439, 2026-11-18)

- On `20aa17caf09d99acb0bc8b3cd69b9e7fb322fb63`, `npm run test:unit` passed
  (39 tests). The requested `npm run typecheck` and `npm run test` scripts are
  still undefined.
- The full Go suite was not rerun: `SANDCASTLE_VERIFY` is unset in this
  worktree, so the required verification wrapper is unavailable. The previous
  recorded `go test ./...` result remains the latest Go evidence.
- Issue #439 remains partial. The route and state gaps, unavailable wire-byte
  metrics, and no-budget decision remain as recorded above.

## UI identity measurement evidence (#439, 2026-11-18)

- Reused the isolated #449 account, authentication and public-screen evidence
  in [identity measurement](ui-measurement-identity.md). It records cold/warm
  navigation for setup, login, password recovery, account, security, sessions,
  notifications and unavailable public share, plus invalid-login and password
  recovery feedback timings.
- The evidence remains partial: MFA challenge and enrollment, valid public
  share, consent, session revocation, notification delivery, and additional
  validation/error/permission states remain unmeasured. No budget is proposed.
- `npm run test:journeys` passed 112 tests with 2 skipped on this issue branch.
  This is behavior evidence only. The reproducible identity measurement command
  is `UI_MEASURE=identity ./scripts/playwright-smoke.sh`; the home baseline uses
  `UI_MEASURE=1`. Coverage gaps and the no-budget decision remain in
  [identity measurement](ui-measurement-identity.md).

## UI core list measurement evidence (#439, 2026-11-18)

- Added `UI_MEASURE=core ./scripts/playwright-smoke.sh` for reproducible cold
  and warm empty Items, Clients and Files navigation, synthetic Item creation,
  and empty-title browser validation. One run passed on commit
  `4492d2c86536e6f528c7bb9a73287d568ee081ff`; exact values and conditions are
  in [UI measurement](ui-measurement.md).
- Cold list navigations each reported the favicon 404. Items and Clients had
  one unfinished live stream per navigation. The single run sets no budget.
- Populated states, Item edit/server validation, Client detail, member and
  invitation flows, upload/download transfer, permission/no-JavaScript states,
  and repeated-run variance remain unmeasured. Issue #439 remains partial.

## UI measurement rerun on issue branch (#439, 2026-11-18)

- Repeated core, administration, and identity isolated Playwright measurements
  on `132086464fa822f5b2510de4eb061ec3317d5fc5`. Exact commands, current
  interaction results, environment, and known limitations are recorded in the
  linked [UI measurement](ui-measurement.md), [administration evidence](ui-measurement-admin.md),
  and [identity evidence](ui-measurement-identity.md).
- Cold navigations again reported a favicon 404. Export transfer bytes remain
  unavailable; measured states remain partial. No budgets or fixes are claimed.
- `npm run typecheck` and `npm run test` are undefined. `SANDCASTLE_VERIFY` is
  unset, so required wrapped full-suite checks were unavailable. Issue #439
  remains partial; uncovered routes and state gaps are mapped in the evidence.

## UI members and invitations measurement (#439, 2026-11-18)

- Added `UI_MEASURE=members ./scripts/playwright-smoke.sh` for cold/warm
  Members and Invitations navigation and synthetic invitation creation.
  One isolated run passed; exact counts and timings are in
  [UI measurement](ui-measurement.md).
- The cold pages again reported one favicon 404. The invitation URL stayed in
  the browser and was not emitted. Member changes, acceptance and failure,
  permission and no-JavaScript states remain unmeasured. No budget is proposed.
- `npm run test:unit` passed 39 tests. `npm run typecheck` and `npm run test`
  are undefined. `SANDCASTLE_VERIFY` is unset, so wrapped full suites could not
  run. Issue #439 remains partial.

## UI baseline current follow-up (#439, 2026-11-18)

- Repeated the isolated members/invitations runner on
  `9fc03f2c6312086167949344bd97d91c837a3a20`; cold/warm navigation and
  invitation-creation values are recorded in [UI measurement](ui-measurement.md).
- The same-commit core runner reached upload but Chrome closed the page before
  observing its POST. No aggregate results were emitted; upload metrics remain
  unavailable and the failure needs diagnosis.
- `npm run test:unit` passed 39 tests and `git diff --check` passed. The
  requested npm `typecheck` and `test` scripts are undefined. `SANDCASTLE_VERIFY`
  is unset, so wrapped full-suite checks could not run. Issue #439 remains
  partial; route, state and transfer-metric gaps remain in the evidence.

## UI upload measurement follow-up (#439, 2026-11-18)

- Fixed the core measurement locator to target the upload submit button. The
  isolated 256 KiB upload returned 303 with 7 requests, 128,354 encoded bytes,
  1 redirect, zero transport failures and 89.6 ms feedback. Exact environment,
  command and list observations are in [UI measurement](ui-measurement.md).
- Upload failure, progress timing, download transfer, permission and
  no-JavaScript performance states remain unmeasured. Issue #439 remains
  partial; no budget or product fix is claimed.
- Passed: `node --check scripts/ui-measurement-core.mjs`,
  `npm run test:unit` (39 tests), and `git diff --check`. The required
  `npm run typecheck` and `npm run test` scripts are undefined, and
  `SANDCASTLE_VERIFY` is unset, so wrapped full-suite checks are unavailable.

## UI upload request-body measurement (#439, 2026-11-18)

- The core runner now measures the synthetic upload request-body size through
  CDP and keeps it separate from completed response bytes. It stores and emits
  no request headers or payloads. Evidence and exact run conditions are in
  [UI measurement](ui-measurement.md).
- On `e4302a4e831dd384bd65aa199530a7784c1f5ef2`, a 256 KiB upload returned
  303 with 262,477 request-body bytes, 128,354 completed response bytes, 7
  requests, 1 redirect and 83.2 ms feedback. These are body bytes, not full
  wire bytes, and one observation sets no budget.
- `npm run test:unit` passed 40 tests; syntax checks and `git diff --check`
  passed. Requested `npm run typecheck` and `npm run test` scripts are absent.
  `SANDCASTLE_VERIFY` is unset, so wrapped full suites were unavailable. Issue
  #439 remains partial; download bytes, upload failure/progress, and route/state
  coverage remain open.

## UI Item edit measurement (#439, 2026-11-18)

- Extended the isolated core runner to edit its synthetic Item and record
  request count, response bytes, request-body bytes, status and feedback time.
  One run returned 200 with one request, 7,273 completed encoded response
  bytes, 119 request-body bytes and 42.2 ms feedback. Full conditions and
  limitations are in [UI measurement](ui-measurement.md). No budget is set.
- Client detail, populated and failure states, permission/no-JavaScript
  coverage, download transfer bytes and other route/state gaps remain. Issue
  #439 remains partial.
- `node --check scripts/ui-measurement-core.mjs`, isolated core measurement,
  `npm run test:unit` (40 tests), and `git diff --check` passed. The requested
  `npm run typecheck` and `npm run test` scripts are absent. `SANDCASTLE_VERIFY`
  is unset, so wrapped full-suite checks are unavailable.
