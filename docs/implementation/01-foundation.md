# Phase 01: Foundation

## Goal

Create the smallest runnable Go foundation that proves explicit composition,
portable persistence, validated configuration, and one shared application
boundary without implementing product features early.

## Standards covered

`STANDARDS.md` Sections 1, 2, 5, 6 "Configuration and secrets," 10, and the
applicable test foundations in Section 13.

## Prerequisites

- Verify the current supported stable Go release from official Go sources and
  record the selected version and support policy.
- Keep decision 0001 accepted or replace it through a reviewed decision.

## Required outcomes

- Establish the Go module, `cmd/server` entry point, one composition root under
  `internal/bootstrap`, vertical feature boundaries, and explicit dependency
  wiring.
- Define typed startup configuration with validation, safe errors, clear
  ownership classes, and no secret values in logs.
- Create ordered embedded migrations and database adapters for persistent
  SQLite and optional PostgreSQL without an ORM or generic repository.
- Configure SQLite foreign keys, busy timeout, safe journaling, and one-instance
  semantics. Define PostgreSQL as the multi-instance path.
- Establish explicit context propagation, transaction ownership, UTC time, and
  portable application-generated identifiers.
- Add a minimal process start/stop path sufficient for tests; do not build the
  final HTTP, identity, or feature behavior assigned to later phases.
- Create the final architecture and configuration documents only after these
  behaviors exist.

## Evidence

- Focused configuration and composition tests.
- Real temporary SQLite migration/repository test.
- Isolated PostgreSQL migration/repository compatibility test.
- `go test ./...`, `go vet ./...`, and `go test -race ./...` pass.
- Capability rows for architecture, configuration, and database foundations
  link to real evidence.

## Exclusions

No DI container, ORM, generic repository, broker, cache, object store,
microservice, SPA runtime, internal AI, or product-specific abstraction.

## Completion gate

Both databases apply the same ordered history, startup rejects invalid
configuration without exposing secrets, dependencies assemble explicitly, and
all evidence above is linked from `docs/capabilities.md`.

## Completion evidence

Completed 2026-08-25. Go 1.27.0 test, vet, race, isolated PostgreSQL 14.24,
SQLite migration, and process startup/shutdown checks pass. Evidence links live
in `docs/capabilities.md`.
