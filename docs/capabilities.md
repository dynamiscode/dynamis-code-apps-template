# Capability and Conformance Ledger

This ledger records implemented conformance evidence and deferred triggers.

Dated evidence sections are append-only. New verification is recorded as a new
dated section; past dated sections are never rewritten, because each is a record
of what was verified on its date.

## Profile status

| Profile | Status | Evidence |
|---|---|---|
| Core | conforming | [Foundation through delivery evidence](#phase-07-evidence) |
| Identity | conforming | [Phase 02 evidence](#phase-02-evidence) |
| Agent | conforming | [Phase 05 evidence](#phase-05-evidence) |
| Files | conforming | [Files evidence](#files-evidence) |
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
| Light, dark, and system appearance | bootstrap, recurring | 02, 04, 06 | conforming | [Theme contract](web.md#theme), [theme tests](../internal/web/theme_test.go), [web tests](../internal/web/handler_test.go) |
| REST and optional Agent MCP/remote CLI interfaces | bootstrap, recurring | 03-05, 09 | conforming | REST: [OpenAPI](../api/openapi.json), [HTTP contracts](../internal/httpapi/handler_test.go), [SCIM contracts](../internal/httpapi/scim_test.go); [MCP tests](../internal/mcpserver/server_test.go); [CLI tests](../internal/appctl/run_test.go) |
| Local authentication, OIDC, MFA, and passkey service | bootstrap, recurring | 02 | conforming | [Authentication](authentication.md), [MFA service](../internal/identity/mfa.go), [OIDC tests](../internal/identity/oidc_test.go), [configuration tests](../internal/platform/config/config_test.go) |
| Optional MCP OAuth protected-resource metadata | bootstrap, recurring | 05 | conforming, off by default; trigger accepted in [decision 0007](decisions/0007-mcp-oauth-protected-resource-metadata.md) | [MCP discovery](mcp.md#protected-resource-discovery), `internal/mcpserver/oauth_test.go`, `internal/bootstrap/agent_smoke_test.go` (Agent profile only), [configuration tests](../internal/platform/config/config_test.go) |
| Built-in MCP OAuth authorization server | bootstrap, recurring | 05 | implemented behind `MCP_OAUTH_ENABLED`; expired-record pruning and accessibility-smoke evidence pending | [decision 0008](decisions/0008-built-in-mcp-authorization-server.md), [MCP OAuth](mcp.md#built-in-authorization-server), `internal/identity/mcp_oauth_test.go`, `internal/mcpserver/oauth_token_test.go`, [configuration tests](../internal/platform/config/config_test.go) |
| Permissions, roles, workspaces, invitations, sessions, tokens, SCIM provisioning, account lifecycle, preferences, and notifications service | bootstrap, recurring | 02, 09 | conforming | [Authorization and lifecycle tests](../internal/identity/service_test.go), [SCIM lifecycle tests](../internal/identity/scim_test.go), [account and notification tests](../internal/identity/account_notification_test.go), [PostgreSQL identity test](../internal/identity/postgres_test.go) |
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
| Audit events | bootstrap, recurring | 02, 04, 06 | conforming | [Identity audit tests](../internal/identity/audit_test.go), [browser audit tests](../internal/web/handler_test.go), [retention tests](../internal/platform/maintenance/maintenance_test.go), [access/deletion rules](data-lifecycle.md), [decision](decisions/0006-workspace-audit-history.md) |
| Workspace webhooks | triggered, recurring | 08-09 | conforming | [Decision](decisions/0004-webhooks.md), [REST contract](api.md#webhooks), [browser contract](web.md), [service tests](../internal/webhooks/service_test.go), [browser tests](../internal/web/webhooks_test.go), [operations](operations.md#webhook-delivery) |
| Background jobs | triggered, recurring | 09 | conforming | [Decision](decisions/0005-background-jobs.md), [queue tests](../internal/jobs/queue_test.go), [migration](../internal/platform/database/migrations/000010_background_jobs.sql), [operations](operations.md#health-telemetry-and-limits) |
| Bounded public Item sharing | triggered, recurring | 09 | conforming | [Decision](decisions/0005-public-sharing.md), [web contract](web.md), [sharing tests](../internal/sharing/service_test.go), [browser tests](../internal/web/handler_test.go), [lifecycle](data-lifecycle.md) |
| Backup, restore, upgrades, RPO, and RTO | operational | 06 | conforming | [SQLite/PostgreSQL restore tests](../internal/platform/backup/backup_test.go), [operator procedures](operations.md#backup-and-restore) |
| WCAG 2.2 AA accessibility | bootstrap, recurring | 04, 07 | conforming | [Automated runner](../scripts/accessibility.mjs), [dated automated and manual evidence](accessibility.md) |
| Containers and deployment | bootstrap, operational | 07 | conforming | Pinned [image](../Dockerfile), [SQLite Compose](../compose.yaml), [PostgreSQL overlay](../compose.postgres.yaml), [smoke](../scripts/docker-smoke.sh), and [deployment contract](deployment.md) |
| CI, release security, SBOM, provenance, signatures, checksums | operational | 07 | conforming | [CI](../.github/workflows/ci.yml), [source security workflow](../.github/workflows/source-security.yml), [release workflow](../.github/workflows/release.yml), [dependency monitoring](../.github/dependabot.yml), [Scorecard](../.github/workflows/scorecard.yml), and [artifact verification](release.md) |
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

## OAuth token short ID evidence

Ticket #247 moves `oauth_tokens` to an integer key with the `oauth_token` short ID, and
converts `parent_id` to match. Token secrets and their `secret_hash` lookups are unchanged,
and an integer, decimal, or short row ID is rejected as a bearer token by access
authentication, refresh, and revoke. The principal's token ID is the short ID. The exception
is [2026-11-12-oauth-tokens-short-id-migration-rewrite](decisions/exceptions/2026-11-12-oauth-tokens-short-id-migration-rewrite.md).
Verification results are recorded in the pull request for ticket #247.

## Session and audit short ID evidence

Ticket #248 moves `sessions` and `audit_events` to integer keys with the `session` and
`audit_event` short IDs, and converts `mfa_challenges.session_id` and `oauth_grants.session_id`
to `BIGINT`. The session cookie value and its `secret_hash` lookups are unchanged. An integer,
decimal, malformed, or foreign-type ID is rejected as the session secret, as a CSRF session,
and as a revoke target, and gets the same invalid-session problem as an unknown session. Session
audit targets and exported audit event IDs are short IDs. Both tables use `AUTOINCREMENT` on
SQLite, so a deleted maximum key is never reused. The exception is
[2026-11-12-session-audit-short-id-migration-rewrite](decisions/exceptions/2026-11-12-session-audit-short-id-migration-rewrite.md).
Evidence: `TestSQLiteSessionAndAuditTablesUseIntegerPrimaryKeys`,
`TestSQLiteSessionAndAuditKeysAreNeverReused`, `TestPostgresSessionAndAuditTablesUseIntegerPrimaryKeys`,
`TestSessionShortIDsAreExposedAndNeverAccepted`, `TestIdentityRESTContracts`, and the
maintenance and portability tests. `make lint` and `go generate ./api` show no drift.


## User and external identity short ID evidence

Ticket #249 moves `users` and `external_identities` to integer keys with the `user` and
`external_identity` short IDs. Every text `users(id)` foreign key (21 columns) becomes `BIGINT`.
`oidc_transactions.state_hash` stays hash-keyed. Both tables use `AUTOINCREMENT` on SQLite, so a
deleted maximum key is never reused. REST and MCP responses that carry user IDs expose the user short
ID. The exception is
[2026-11-12-user-identity-short-id-migration-rewrite](decisions/exceptions/2026-11-12-user-identity-short-id-migration-rewrite.md).
Evidence: `TestSQLiteUserTablesUseIntegerKeysAndForeignKeys`, `TestSQLiteUserAndExternalIdentityKeysAreNeverReused`,
`TestPostgresUserTablesUseIntegerKeysAndForeignKeys`, and the identity, httpapi, MCP, SCIM, and
session tests. `make lint` and `go generate ./api` show no drift.

## Notification and account token short ID evidence

Issue #245 moves notification keys to integers and exposes only the ADR 0009 short
ID in web JSON, SSE events, and the `after` cursor, using the type code
`notification`. Email verification and password reset rows take integer keys with
the type codes `email_verification` and `password_reset`. Their secrets stay random,
and lookups stay on `token_hash`, so a row's integer key or short ID is never accepted
as the secret. A malformed notification cursor resyncs, and a malformed mark-read ID
returns the same denial as an unknown one. The exception is
[2026-11-12-notification-account-token-short-id-migration-rewrite](decisions/exceptions/2026-11-12-notification-account-token-short-id-migration-rewrite.md).
Verified 2026-11-12 with Go 1.27.4 and PostgreSQL 14.24: focused tests for the type-code
registry, notification cursor and mark-read rejection, and account token row keys pass on
SQLite and PostgreSQL; `make test` passes on SQLite; `go test -p 1 -count=1 ./...` passes on
a fresh PostgreSQL 14.24 container; `make lint` and `go generate ./api` show no drift.

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

## MFA short ID evidence

Verified 2026-11-12. Ticket #246 of spec #243 gives MFA challenges, recovery codes,
and passkeys integer keys under the `mfa_challenge`, `mfa_recovery_code`, and
`mfa_passkey` type codes. Passkey IDs in list and removal responses are short IDs;
`mfa_passkeys.credential_id` stays text. The challenge cookie still carries the
token, and lookups stay on `token_hash` and `code_hash`. Migration 000015 is
rewritten in place under the [recorded exception](decisions/exceptions/2026-11-12-mfa-short-id-migration-rewrite.md).

- Covered: `TestMFARowIDsAreNotCredentials` (SQLite) shows that challenge and
  recovery-code row IDs, as integers or short IDs, are rejected as the bearer
  secret, that the real recovery code still works after those probes, and that a
  malformed or integer passkey ID gets the same error as a missing passkey. `TestSQLiteMFATablesUseIntegerPrimaryKeys`
  and `TestPostgresMFATablesUseIntegerPrimaryKeys` check the key types;
  `TestSQLiteMFAChallengeKeysAreNeverReused` checks that a deleted maximum challenge
  key is not reused on SQLite; and the
  expired-challenge audit test checks that audit targets carry the short ID.
- Checks run: `make lint` passed; `make generate-check` produced no drift;
  `make test-postgres` passed for `internal/identity` and `internal/files` against
  PostgreSQL 14.24; the `POSTGRES_TEST_URL` run against a fresh PostgreSQL 14.24
  container passed for `internal/platform/database` and `internal/platform/maintenance`.
  `make test` passed on SQLite for all packages.
- Not covered: a real WebAuthn enrollment and passkey list, since the test harness
  has no WebAuthn ceremony; the passkey REST routes have no short-ID probe test of
  their own beyond the service-level check.
- Known limitation: SQLite `INTEGER PRIMARY KEY` without `AUTOINCREMENT` reuses the
  key of a deleted maximum row, and retention deletes expired challenges. A reused
  challenge key is not a credential, but an older audit event may name a newer
  challenge. PostgreSQL identities never reuse keys.

## Files evidence

Files metadata migration and workspace authorization, local filesystem uploads
and streams, private S3-compatible presigned PUT/GET paths, configurable
16 MiB object and 1 GiB workspace limits, safe filename and content allowlists,
browser ordinary-form fallback, REST/OpenAPI behavior, and generated profile
pruning are covered by focused SQLite tests and generated-app checks. Deletion,
reconciliation, scanning, background jobs, and PostgreSQL/Docker execution
remain follow-up verification or deferred behavior for this slice.

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
- not yet run: `make accessibility-smoke` for the consent page
- not yet implemented: expired-record pruning

## Object storage S3 conformance evidence

Verified 2026-11-11 with Go 1.27.4 on the in-process conformance suite:

- `TestS3ConformanceInProcess` (`internal/files/s3_conformance_test.go`) runs
  the S3 object contract through the production `newS3Store` constructor against
  an in-process S3-compatible endpoint. It covers a put/head/get round trip with
  size and content type, conditional create rejecting overwrite, not-found
  normalized to `ErrObjectNotFound` for head and get, a presigned PUT using the
  returned headers, a presigned GET whose `X-Amz-Expires` equals the requested
  TTL, unsigned requests refused so objects stay private, and a presigned GET
  refused after expiry
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

Manual R2 run: **not run**. It is gated on R2 credentials and a dedicated test
bucket, neither of which was available to this run. Procedure:
[deployment](deployment.md#cloudflare-r2-object-storage). Record the result as a
new dated section here: date, endpoint kind (no account ID), Go version, and
`go test` outcome.

Not verified against R2 until that run passes: SigV4 signature validity for
presigned URLs, the SDK's default request checksum behavior, conditional PUT
with `If-None-Match`, and browser CORS preflight for presigned uploads.

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
