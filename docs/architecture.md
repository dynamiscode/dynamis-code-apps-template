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
    -> web / REST adapters
    -> shared identity and item use cases
    -> SQLite or PostgreSQL adapters

optional Agent profile
    -> MCP adapter
    -> REST-only remote CLI

cmd/appctl -> remote REST API only (Agent profile)
```

Business features use vertical slices. Application packages own rules and
transactions. Web, REST, and optional MCP call the same item and identity use
cases. The CLI calls the remote REST API only. WebMCP is a browser-tab-bound
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
UTC timestamps live in `schema_migrations`. Integer primary keys are the one
dialect-specific fragment: the runner renders `{{integer_pk}}` per driver.
Migrations 000002 through 000016 and 000018 were rewritten in place for workspace,
member, invitation, API token, item, webhook, and job short IDs under recorded
exceptions: [workspaces, members, invitations, API tokens, and items](decisions/exceptions/2026-11-11-workspace-short-id-migration-rewrite.md)
and [webhooks and jobs](decisions/exceptions/2026-11-11-webhook-job-short-id-migration-rewrite.md),
accepted by the repository owner on 2026-11-12.
Migrations 000011, 000012, 000013, and 000016 were rewritten again for sharing, SCIM,
and MCP OAuth record keys under [recorded exception](decisions/exceptions/2026-11-11-sharing-scim-oauth-short-id-migration-rewrite.md).
Upgrades use a documented
stop-the-world procedure; SQLite snapshots and PostgreSQL dumps have separate
verified restore paths.

## IDs, time, and transactions

`internal/platform/id` creates portable 128-bit opaque IDs with `crypto/rand`.
Persisted timestamps use UTC RFC 3339 with nanosecond precision. Application
use cases own transactions spanning multiple data operations; repositories do
not start hidden transactions.

Workspaces are the exception to opaque IDs. They use an integer key
(`INTEGER PRIMARY KEY` on SQLite, `BIGINT GENERATED ALWAYS AS IDENTITY` on
PostgreSQL). `identity.WorkspaceID` marshals to its short ID, the only form that
leaves the process. Items use the same pattern with `items.ItemID` and the item
type code. Webhooks, webhook deliveries, webhook events, and background jobs use
integer keys too; `internal/webhooks/ids.go` marshals the first three to their
short IDs under their own type codes. The short-ID codec is [ADR 0009](decisions/0009-short-id-codec.md).

## Domain state machines

Domain entities with non-trivial lifecycles (such as invitations, jobs, webhooks,
and uploads) model state transitions explicitly rather than relying on implicit
database column nullability checks:

1. **Typed status enum**: Define an explicit string-backed status type
   (e.g., `InvitationStatus`) with named constants for all lifecycle states
   (`pending`, `accepted`, `revoked`, `expired`).
2. **Pure computed status**: Implement a pure method on the model
   `func (m Model) Status(now time.Time) StatusEnum` that evaluates timestamps
   and precedence to compute current status dynamically without side effects.
3. **Canonical transition validator**: Implement a pure transition function
   `func Validate<Resource>Transition(current, target StatusEnum) error`
   enforcing allowed state changes and returning wrapped domain errors
   on illegal transitions.
4. **Service enforcement**: Application services load current status, validate
   the transition before mutating state, execute atomic database updates, and
   record matching audit events.

## Go support

As verified on 2026-08-25, current stable Go is 1.27.0. The module keeps Go
1.26 as language baseline and records Go 1.27 as development toolchain, matching
Go's two-release support window. Version sources:
[Go downloads](https://go.dev/dl/) and
[Go release policy](https://go.dev/doc/devel/release).

OpenTelemetry instrumentation runs without requiring a backend. Bounded
workspace export, retention maintenance, backup/restore, and the database-backed
background-job queue remain platform services; no broker, cache, or
observability stack is bundled.

## Asynchronous state machines

Background queues, workers, and delivery state transitions use pure outcome
evaluators (`EvaluateJobOutcome`, `evaluateDeliveryOutcome`) with explicit typed
status enums (`JobPending`, `JobRunning`, `JobSucceeded`, `JobFailed`;
`DeliveryPending`, `DeliveryDelivered`, `DeliveryFailed`). Handlers and
persistence queries apply the evaluated outcome rather than branching on
attempt counts or lease boundaries inline. State transitions are verified
across attempt boundaries (initial attempt, retry backoff, exhaustion, and
stale lease reclamation) with unit tests before persistence updates.

## Profile composition

Generated applications require Core and Identity. Identity remains included
because Core web, REST, item, and bootstrap paths use its shared authentication
and workspace-authorization services. Agent is optional: when selected, the
generator retains the MCP server, its bootstrap route, the REST-only `appctl`
command, and their tests. Files is optional and requires Core plus Identity. It
adds workspace-scoped private file metadata with explicit state transitions
(Pending -> Ready, Pending -> Failed), local or S3-compatible object storage,
quota reservations across active and failed uploads, and browser/REST surfaces.
CompanySite and Integrations remain unimplemented.
