# Phase 03: HTTP and REST

## Goal

Build one bounded HTTP foundation and a stable REST contract that transports
shared use cases without owning domain rules.

## Standards covered

`STANDARDS.md` Section 3 REST requirements; Section 6 HTTP foundation, health,
REST behavior, and contract lifecycle; applicable Section 7 changes; and HTTP
acceptance in Section 13.

## Prerequisites

Phase 01 composition/configuration and the Phase 02 authorization boundary are
complete.

## Required outcomes

- Build one HTTP server and middleware chain with validated request IDs,
  structured correlation, header/body/time limits, graceful shutdown, and
  application-specific security headers, including the WebMCP boundary
  headers `Permissions-Policy: tools=(self)` and `Origin-Agent-Cluster: ?1`.
- Expose distinct liveness and bounded database-backed readiness contracts.
- Define public `/api/v1` operations in OpenAPI 3.1 and make generation
  reproducible through `go generate ./api`.
- Return RFC 9457 Problem Details with stable types, codes, request IDs, safe
  detail, and no internal or secret data.
- Standardize cursor pagination, stable ordering, allowlisted filters/sorts,
  maximum page size, and indexes.
- Define ETags and conditional mutations where caching or concurrent overwrite
  matters.
- Define idempotency keys for retryable or side-effectful creates, including
  principal/operation scope, request matching, conflict, and retention.
- Apply stricter abuse controls to authentication endpoints and documented
  `429` plus `Retry-After` behavior.
- Publish compatibility and deprecation rules for REST and generated clients.
- Define bearer-authenticated REST contracts for members, ownership, invitations,
  tokens, and sessions using the Phase 02 permission boundary. Do not add REST
  workspace listing or creation endpoints.
- Create final API documentation only after the contract exists.

## Evidence

- HTTP component tests cover time/body limits, request IDs, security headers,
  liveness, readiness, authorization, rate limits, and safe internal failures.
- Contract tests cover Problem Details, pagination, unsupported filters,
  conditional writes, and idempotency replay/conflict.
- `go generate ./api` is reproducible with no unexplained diff.
- Full Go test, vet, and race gates pass.

## Exclusions

No GraphQL, gRPC, arbitrary version negotiation, custom client error format,
feature-local middleware, or duplicated authorization in handlers.

## Completion gate

The API contract, server behavior, generated artifacts, component tests, and
documentation agree, and distinct health semantics work against a real
database adapter.
