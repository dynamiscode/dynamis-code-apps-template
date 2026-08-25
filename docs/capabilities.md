# Capability and Conformance Ledger

This ledger reports current evidence. The template is not implemented, so no
planned capability is conforming yet.

## Profile status

| Profile | Status | Evidence |
|---|---|---|
| Core | pending | Phases 01, 03, 04, 06, and 07 |
| Identity | pending | Phase 02 |
| Agent | pending | Phase 05 |
| Production | not applicable | No deployment serves real users or durable data |

`pending` is valid only during generation. `Core`, `Identity`, and `Agent`
must contain no pending groups before `STANDARDS.md` is deleted.

## Required standard groups

| Standard group | Lifecycle | Phase | Status | Evidence target |
|---|---|---:|---|---|
| Purpose, modular-monolith boundaries, scaling path | bootstrap, recurring | 01 | conforming | [Architecture](architecture.md), [composition test](../internal/bootstrap/app_test.go) |
| Web, REST, MCP, and remote CLI interfaces | bootstrap, recurring | 03-05 | pending | Shared-use-case tests and interface contracts |
| Optional WebMCP browser enhancement | triggered, recurring | 04, 07 | pending | Browser registration, redaction, fallback, security-header, and smoke evidence |
| Local authentication and OIDC | bootstrap, recurring | 02 | pending | Identity tests and authentication docs |
| Permissions, roles, workspaces, invitations, sessions, and tokens | bootstrap, recurring | 02 | pending | Authorization matrix and isolation tests |
| SQLite, PostgreSQL, migrations, and rolling compatibility | bootstrap, recurring | 01, 06 | pending | Phase 01: [database implementation](../internal/platform/database/), real SQLite and PostgreSQL tests; rolling compatibility remains Phase 06 |
| Data governance, portability, deletion, archive, and restoration | bootstrap, operational | 06 | pending | Policies, use cases, and lifecycle tests |
| Configuration and secrets | bootstrap, recurring | 01 | conforming | [Configuration](configuration.md), [validation tests](../internal/platform/config/config_test.go), safe `.env.example` |
| HTTP limits, request IDs, timeouts, headers, and abuse controls | bootstrap, recurring | 03 | pending | HTTP component tests |
| Traces, metrics, logs, health, shutdown, and operational targets | bootstrap, operational | 06 | pending | Telemetry and lifecycle tests |
| RFC 9457, collections, conditional writes, and idempotency | bootstrap, recurring | 03 | pending | OpenAPI and contract tests |
| Contract lifecycle and deprecation | recurring | 03 | pending | Compatibility policy and contract checks |
| Long-running operations | bootstrap, triggered | 06 | pending | Operation state and authorization tests |
| Realtime delivery | bootstrap, recurring | 04 | pending | SSE reconnect, scope, and limit tests |
| Quotas and resource limits | bootstrap, operational | 06 | pending | Limit enforcement and observability tests |
| Audit events | bootstrap, recurring | 02, 06 | pending | Redaction, append-only, and access tests |
| Backup, restore, upgrades, RPO, and RTO | operational | 06 | pending | Automated isolated restore evidence |
| WCAG 2.2 AA accessibility | bootstrap, recurring | 04, 07 | pending | Automated and manual critical-flow evidence |
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
