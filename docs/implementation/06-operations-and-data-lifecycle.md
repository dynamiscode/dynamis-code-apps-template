# Phase 06: Operations and Data Lifecycle

## Goal

Make the implemented application observable, bounded, recoverable, and honest
about durable-data lifecycle before production delivery work begins.

## Standards covered

`STANDARDS.md` Sections 5 and 6 for rolling compatibility, governance,
portability, deletion/restoration, observability, lifecycle, long-running
operations, quotas, audit, backup/restore, and operational targets.

## Prerequisites

Phases 01-05 provide real identity, feature, HTTP, realtime, MCP, CLI, and both
database paths to observe and recover.

## Required outcomes

- Add OpenTelemetry-compatible traces and metrics plus structured log
  correlation without requiring a bundled backend.
- Measure request rate/errors/duration, authentication failures, database
  health, streams, and background delivery when present. Keep secrets and
  unrelated personal data out of telemetry.
- Bound pools, streams, request queues, operations, sessions, storage, and
  external calls at the smallest meaningful scope with documented errors.
- Complete append-only audit storage, access, retention, export, and deletion
  behavior.
- Define a durable long-running-operation resource only for work that outlives
  ordinary requests; include scope, states, retry, cancellation, idempotency,
  cleanup, and audit.
- Classify persisted fields and define purpose, retention, export, correction,
  deletion/anonymization, and backup treatment.
- Implement authorized versioned workspace export. Define an import path for
  cloud/self-hosted portability or explicitly document why import is not
  supported; any import treats input as untrusted and cannot elevate authority.
- Define archive, soft-delete, permanent-delete, and restore semantics for the
  sample resource and identity/workspace lifecycle.
- Expose authorized workspace export through the browser while retaining
  operator-only backup, restore, maintenance, telemetry, and lifecycle controls.
- Prepare the user export link for optional WebMCP without returning export
  content or secret data to the browser agent. User activation continues
  through the existing authorization, audit, and telemetry path.
- Make migrations interruption-safe and document expand-contract or explicit
  stop-the-world upgrades.
- Implement separate consistent SQLite and PostgreSQL backup/restore paths,
  checksums, isolated automated restore verification, key recovery, and chosen
  RPO/RTO ownership.
- Create final operations and deployment documentation for implemented
  behavior.

## Evidence

- Telemetry tests prove correlation and redaction; readiness uses bounded real
  dependency checks.
- Limit, audit, operation-state, export/import where applicable, deletion, and
  restoration tests prove authorization and workspace isolation.
- Both database restore tests recover known temporary records and reject stale
  or corrupt evidence.
- Upgrade tests exercise migration interruption and the documented
  compatibility strategy.
- Measured idle/load resource use and operational alert targets are recorded.
- Full Go test, vet, race, and database compatibility gates pass.

## Exclusions

No bundled observability backend, cache, broker, object storage, job system,
analytics service, or production data copied into fixtures.

## Completion gate

Operators can detect failure, bound consumption, explain data lifecycle, and
restore both supported databases within documented objectives using verified
procedures.
