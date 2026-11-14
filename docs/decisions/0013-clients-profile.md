# 0013: Clients profile composition, lifecycle, and surfaces

- Status: accepted
- Date: 2026-11-13
- Owners: template maintainers

## Context

Businesses on this template manage clients. Every workspace needs the same
common client fields, and each workspace also needs its own custom fields on
clients. Storage for custom values is decided in [ADR 0012](0012-client-custom-field-storage.md).

The template composes optional profiles. `Files` requires Identity. `Agent`
adds the MCP server, the REST-only `appctl` client, a bootstrap route, and an
Agent smoke test ([Template lifecycle](../template-lifecycle.md)).

## Decision

### Profile and surfaces

- `Clients` is an optional profile. It requires Identity and Agent.
- It ships beside the sample Item. Generated applications may remove Item as
  they do today.
- v1 exposes web, REST, MCP, and `appctl`. Each calls the shared application
  use cases; none accesses SQL directly ([MCP](../mcp.md)).

### Records and identifiers

- **Client:** workspace-scoped. Short ID with type code `client`.
- **Common fields:** display name (required), email, phone, notes, and
  `archivedAt` (date-time, nullable). The template fixes them in code and
  migrations. Workspaces cannot add, remove, or rename them.
- **Custom field:** workspace-defined. Short ID with type code `client_field`.
  Type is one of text, number, date, yes/no, or single choice.
- **Choice option:** a selectable value of a single-choice field.

### Roles

- Owners and admins define, archive, and delete custom fields and choice
  options, and permanently delete clients.
- Members create and edit clients and their custom values.
- Viewers read.

### Querying

- Custom fields filter and sort for all five types.
- Operators: text equals or contains (case-insensitive); number and date equals,
  before, after, between; yes/no is true or false; single choice equals one or
  more options.
- Sort uses one field at a time, with empty values last.

### Lifecycle

- **Client archive** sets `archivedAt`. Archived clients drop out of the default
  list and stay reachable through a filter.
- **Client permanent delete** (owner or admin) removes the client and its
  custom values in the request's transaction.
- **Field archive** hides the field from forms and keeps its values. Archive is
  reversible.
- **Field permanent delete** is allowed only for an archived field. It clears
  the field's values from every client and frees its slot (ADR 0012). The
  field's type locks after the first value is stored.
- **Choice option delete** is immediate. It clears the option from client
  values in the request's transaction.

### Labels

- Common field labels come from the locale catalog and follow the workspace
  locale ([ADR 0003](0003-localization.md)).
- Custom field labels are workspace-entered text and show as typed.

### Audit, export, events

- Client and field-definition changes write audit events
  ([ADR 0006](0006-workspace-audit-history.md)).
- The workspace export includes clients, field definitions, and values.
- Webhooks: `client.created`, `client.updated`, `client.archived` (when
  `archivedAt` is set), and `client.deleted`. They go through the existing
  outbox ([ADR 0004](0004-webhooks.md), [ADR 0005](0005-background-jobs.md)).
  Payloads carry the client's short ID, its common fields, and its custom
  values keyed by field short ID. Field-definition changes emit no events.
- Realtime: client list pages refresh over workspace-scoped SSE, as Items do
  ([Web realtime contract](../web.md#realtime-contract)).

## Consequences

Benefits:

- A single profile gives every workspace the same common fields, with
  per-workspace custom fields.
- Web, REST, MCP, and `appctl` share one set of rules.

Costs:

- Every application with Clients also serves MCP and `appctl`, because Clients
  requires Agent.
- Clearing values is synchronous with no client cap (see ADR 0012).
- Four webhook events and the realtime list add public contract surface that
  later changes must keep compatible.

Rejected alternatives:

- Clients usable without Agent: MCP and `appctl` would then appear only when
  Agent is also composed. Rejected in design review.
- Web and REST only in v1: deferred, so MCP and `appctl` come later. Rejected in
  design review.
- Webhooks for field-definition changes: not needed for v1 integrations.

## Revisit when

- Integrators need MCP or `appctl` without the Agent profile.
- Synchronous clearing exceeds the request budget on a measured workspace.
- Integrators need field-definition events.
