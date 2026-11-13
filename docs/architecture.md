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
Migration 000007 was rewritten in place for the notification key under [recorded exception](decisions/exceptions/2026-11-12-notification-account-token-short-id-migration-rewrite.md).
Migration 000015 was rewritten in place for the MFA passkey key
under [recorded exception](decisions/exceptions/2026-11-12-mfa-short-id-migration-rewrite.md).
Migrations 000002, 000003, 000005, 000007, 000012, 000013, 000014, 000015, 000016,
and 000020 were rewritten in place for the user key (`BIGINT` user references and
the `user` short ID) under [recorded exception](decisions/exceptions/2026-11-13-users-short-id-migration-rewrite.md),
which is `accepted`.
Upgrades use a documented
stop-the-world procedure; SQLite snapshots and PostgreSQL dumps have separate
verified restore paths.

## IDs, time, and transactions

Entity records are keyed by integers, and their public IDs are short IDs from
`internal/platform/id` ([decision 0011](decisions/0011-identity-integer-keys.md),
[ADR 0009](decisions/0009-short-id-codec.md)). `id.New()` creates 128 random bits
from `crypto/rand`, hex-encoded, for credentials and protocol state only, such as
session, CSRF, OAuth, MFA, and verification values. Those values are never entity IDs.
Persisted timestamps use UTC RFC 3339 with nanosecond precision. Application
use cases own transactions spanning multiple data operations; repositories do
not start hidden transactions.

Workspaces use an integer key (`INTEGER PRIMARY KEY` on SQLite,
`BIGINT GENERATED ALWAYS AS IDENTITY` on PostgreSQL). `identity.WorkspaceID`
marshals to its short ID, the only form that leaves the process. Items use the
same pattern with `items.ItemID` and the item type code. Webhooks, webhook
deliveries, webhook events, and background jobs use integer keys too;
`internal/webhooks/ids.go` marshals the first three to their short IDs under
their own type codes. Audit targets and actors, idempotency principals, and job
deduplication keys are typed integer keys as well; see the decision for the
exact columns and the two open owner decisions.

Files follow the same pattern with an integer key and the `file` type code.
Migration 000020 rebuilds `files` through a new table and renames it, copying `object_key` unchanged so
stored objects stay addressable. SQLite uses `AUTOINCREMENT` for file keys so a
deleted key is never reused; PostgreSQL identities never reuse keys either.
Object keys use an opaque token, not the integer key.

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
