# 0001: Go Modular Monolith

- Status: accepted
- Date: 2026-08-25
- Owners: template maintainers

## Context

The template must start with low operational cost, support web users and
automation, and scale without a premature service split. Its implementation
stack must be concrete enough for agents to use exact paths and checks.

## Decision

Build a Go modular monolith with vertical feature slices, explicit manual
constructor injection, and pragmatic interfaces only at real boundaries.

- Serve HTML on the server; use HTMX for targeted updates and SSE for one-way
  realtime delivery.
- Expose versioned REST described by OpenAPI.
- Expose authenticated MCP tools through shared application use cases.
- Provide a remote CLI that calls REST only.
- Default to persistent SQLite for one instance and keep an optional portable
  PostgreSQL path for multiple instances.
- Prefer the Go standard library and already-selected dependencies.

Do not add a runtime DI container, ORM, generic repository, microservice,
broker, cache, SPA runtime, WebSocket, or extra protocol without a measured
requirement and accepted plan change.

## Consequences

The default deployment stays small and understandable. Shared application
rules prevent interface drift. Database portability and explicit boundaries
cost some up-front tests, but avoid a later architecture rewrite.

## Revisit when

Measurements show that a module needs independent deployment, ownership,
scaling, reliability, or security isolation; HTMX/SSE cannot satisfy a real
interaction; or SQLite/PostgreSQL portability blocks a required feature.
