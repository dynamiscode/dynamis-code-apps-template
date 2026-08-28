# Architecture

This document describes the implemented modular-monolith boundaries.

## Runtime shape

The template is a Go modular monolith. `cmd/server` loads configuration and
calls one composition root in `internal/bootstrap`. Bootstrap opens the
selected database, applies migrations, owns resource shutdown, then waits for
process cancellation.

```text
cmd/server
    -> typed configuration
    -> bootstrap composition root
    -> web / REST / MCP adapters (optional browser WebMCP enhancement)
    -> shared identity and item use cases
    -> SQLite or PostgreSQL adapters

cmd/appctl -> remote REST API only
```

Business features use vertical slices. Application packages own rules and
transactions. Web, REST, and MCP call the same item and identity use cases.
The CLI calls the remote REST API only. WebMCP is a browser-tab-bound
progressive enhancement over the server-rendered web surface; it prepares
visible controls and never becomes a backend or server-MCP transport.

## Dependency rules

- Constructors receive dependencies explicitly. No DI container or service
  locator.
- Interfaces exist only at external boundaries or useful test seams.
- Platform packages cannot import feature transport adapters.
- Request context is passed explicitly; no mutable global request state.
- Go standard library comes first. Database drivers cover the external storage
  boundary absent from the standard library.

## Data and scaling

SQLite is default and limited to one application instance. Startup enables
foreign keys, a 5-second busy timeout, WAL journaling, and one database
connection. The database file belongs on persistent storage.

PostgreSQL uses the same `database/sql` boundary and embedded ordered
migrations. Its pool defaults to four open and two idle connections.
PostgreSQL migrations take a transaction-scoped advisory lock. PostgreSQL is
required before multiple application instances.

Migrations are forward-only and run in one transaction. Applied versions and
UTC timestamps live in `schema_migrations`. Upgrades use a documented
stop-the-world procedure; SQLite snapshots and PostgreSQL dumps have separate
verified restore paths.

## IDs, time, and transactions

`internal/platform/id` creates portable 128-bit opaque IDs with `crypto/rand`.
Persisted timestamps use UTC RFC 3339 with nanosecond precision. Application
use cases own transactions spanning multiple data operations; repositories do
not start hidden transactions.

## Go support

As verified on 2026-08-25, current stable Go is 1.27.0. The module keeps Go
1.26 as language baseline and records Go 1.27 as development toolchain, matching
Go's two-release support window. Version sources:
[Go downloads](https://go.dev/dl/) and
[Go release policy](https://go.dev/doc/devel/release).

OpenTelemetry instrumentation runs without requiring a backend. Bounded
workspace export, retention maintenance, backup/restore, and the webhook
delivery loop remain platform services; no broker, cache, generic job system,
or observability stack is bundled.
