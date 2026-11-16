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

## Addendum: choice-option type code (2026-11-13)

Choice options have their own short-ID type code, `client_field_option`,
registered beside `client` and `client_field`. Options stay in a separate table
keyed to a single-choice field, and integer keys are never exposed. This follows
the decision recorded for spec [#294](https://github.com/dynamiscode/dynamis-code-apps-template/issues/294).

## Addendum: v1 slice scope and omissions (2026-11-13)

- **Search:** v1 clients have no free-text search. The client list pages newest
  first with an opaque cursor. Filtering and sorting arrive with custom fields
  (see Querying above). Search across common fields remains deferred.
- **WebMCP:** the Clients pages omit WebMCP in v1. They are ordinary
  server-rendered forms. No WebMCP tool contract (non-secret schemas or control
  mapping) exists for Clients, so enabling it needs a reviewed contract first.
  Reason recorded per [development](../development.md#feature-completeness) omission rules.
- **Realtime:** the client list refreshes over `GET /workspaces/{workspaceId}/clients/events`
  for create and update, as Items do ([web realtime contract](../web.md#realtime-contract)).
  Archive and delete add their events with their tickets.
- **Not yet implemented:** archive and unarchive, custom fields, permanent
  delete, export, webhook events, MCP tools, and `appctl` commands.
  Each lands in a later ticket and updates [capabilities](../capabilities.md).

## Addendum: choice option storage (ticket #300, 2026-11-13)

- **Table `client_field_options`:** integer key with short-ID type code
  `client_field_option` (D3, no integer keys exposed), `workspace_id` (cascade
  from workspaces), `field_id` (cascade from `client_fields`), `slot` (1 to 100,
  CHECK), `label` (text, trimmed, at most 100 characters), `created_at`, and
  `UNIQUE (field_id, slot)`. An index covers `(workspace_id, field_id, slot)`.
- **Limit:** at most 100 options per single-choice field. The limit is a
  constant, not configuration, because the spec fixes it. The 101st add is
  refused with 409 `resource-limit`. Slots are the lowest free in the field, and
  the list is in slot order.
- **Workspace binding:** the `field_id` foreign key alone does not bind an
  option's workspace to its field. Every option query filters by `workspace_id`
  and `field_id` together. A composite foreign key would need a unique
  constraint on `client_fields (workspace_id, id)`, which this slice does not add.
- **Stored value:** a single-choice value is the option's short ID under the
  field's slot. It is accepted only when it names an option of that field in
  that workspace. Anything else (a label, an integer key, another field's
  option, a malformed ID) is refused as invalid input. Filter and sort (#301)
  compare option short IDs.
- **Delete:** one serializable transaction. It authorizes, finds the option,
  clears its short ID from every client that holds it under the field's slot
  (each cleared client's version bumps and gets a list change), deletes the row,
  and writes audit `client_field_option.deleted` with `clearedClients`. Option
  keys can be reused after a delete (SQLite `INTEGER PRIMARY KEY` without
  AUTOINCREMENT). That is safe because no client holds the deleted short ID after
  commit.
- **Audit:** `client_field_option.created` and `client_field_option.deleted`,
  target type `client_field_option`.
- **Roles:** add and delete need `WorkspaceUpdate`. Reads need `ResourcesRead`.
  Adding to an archived field is refused. Deleting an option of an archived
  field is allowed, so its values can be cleared.
- **Open for later tickets:** the field type change path (`setFieldType`, with no
  surface yet) does not remove options. Permanent field delete (#302) must delete
  a field's options with the field, after it clears their values. Option deletion
  emits no webhook event in this slice. Webhook events arrive in a later ticket.
- **Surfaces in this slice:** REST `GET` and `POST
  /client-fields/{fieldId}/options` and `DELETE
  /client-fields/{fieldId}/options/{optionId}`, plus the custom-fields page and
  the client value select. MCP and `appctl` option commands are not in this
  slice.

## Addendum: list filter and sort (ticket #301, 2026-11-13)

- **Search scope:** v1 has no free-text search on clients, across common or
  custom fields. The list takes filter and sort only, as the spec decided. This
  is recorded here and in the [API](../api.md) and `api/openapi.json`.
- **Operators:** as listed under Querying above, with `before` and `after`
  strict and `between` inclusive. Text matching folds ASCII letters only, in both
  dialects. SQLite `lower()` is ASCII-only without ICU, and PostgreSQL uses
  `translate()` to match, so non-ASCII text compares as typed on both.
- **Text order:** PostgreSQL sorts text with the `"C"` collation, which is byte
  order and matches SQLite. Its default locale collation would order differently.
- **Sort and cursor:** a sort is one field, with empty values last and ties by
  client ID in the sort direction. Every page cursor binds its sort, direction,
  filters, last sort value, and last ID. A cursor from another query is refused.
- **Surfaces:** REST takes `sort` and repeated `filter` parameters. The web list
  has one sort and one filter in a GET form, and a next-page link keeps them.
  MCP (`clients_list_v1`) and `appctl` (`clients list`) take the same sort,
  filter, and archived selector as REST. WebMCP stays omitted, as recorded above.
- **Number ordering (decided):** a number field orders and ranges through `CAST`
  to REAL on SQLite and `::numeric` on PostgreSQL, with no index. The product
  owner chose to amend [ADR 0012](0012-client-custom-field-storage.md) to accept
  this, with a scan ceiling and a revisit trigger, rather than add typed numeric
  indexes. Date and yes/no ranges use the slot index as written, because
  fixed-width ISO dates order the same under any collation.
- **Index use (measured on SQLite):** date equality and date ranges use the slot
  index. Sorts use the slot index only to read the workspace's rows, then sort
  with a temporary B-tree, because nulls-last ordering cannot use the index.
  Case-insensitive text matching and number ranges scan the workspace.
- **Choice sort (known gap):** a single-choice sort orders by the option's
  stored short ID, not its label. Ordering by label needs a stored sort key on
  options, which a later ticket can add.

## Addendum: permanent custom field delete (ticket #302, 2026-11-13)

- **Surfaces:** REST `DELETE /api/v1/workspaces/{workspaceId}/client-fields/{fieldId}`
  (operation `deleteClientField`, 204), and the custom-fields page with a
  `POST .../client-fields/{fieldId}/delete` form. The page shows the control only
  to owners and admins, only for an archived field, behind a confirmation
  (`data-confirm`).
- **Delete:** one serializable transaction. It authorizes `WorkspaceUpdate`, then
  refuses an active field (`ErrFieldNotArchived`, REST 409 `field-not-archived`).
  It clears the field's `fNN` key from every client that holds a value, bumping
  each cleared client's version and recording one change-feed entry per client
  (the same shape option delete uses). It deletes the field's choice options,
  then the field row. The slot is freed only at commit, so a rolled-back delete
  keeps the values, the options, and the slot.
- **Clearing rule:** the cleared set is the clients whose value under the slot is
  non-null. That is the predicate the type lock already uses, so clearing and the
  lock agree. Option delete and field delete share the clearing helper.
- **Audit:** `client_field.deleted`, target type `client_field`, target key the
  field's integer key, metadata `{"clearedClients":N}`. No field values are stored
  in the metadata.
- **Webhooks and realtime:** no webhook event, because field-definition changes
  emit none (see Audit, export, events). Cleared clients refresh through the change
  feed, as option delete does.
- **Omissions:** MCP and `appctl` field delete are not in this slice and arrive
  with the agent-surface ticket.

## Addendum: archive, unarchive, and permanent client delete (ticket #303, 2026-11-13)

- **Roles:** archive and unarchive need `ResourcesWrite`, so members and above
  may use them and viewers are refused. The Roles section above does not name
  client archive, so this slice records members and above, as the spec's
  acceptance criteria say. Permanent client delete needs `WorkspaceUpdate`
  (owners and admins).
- **Archive:** sets `archived_at` and bumps the version. Archiving an archived
  client and unarchiving an active client change nothing: no version bump, no
  change-feed entry, no audit event. Like field archive, the call takes no
  version precondition.
- **Delete:** one serializable transaction. It authorizes, reads the client,
  deletes its row, records one change-feed entry, and writes audit
  `client.deleted`. Custom values live in the row's `custom_values`, so the row
  delete removes them. No other table holds a client's values. An active client
  can be deleted directly.
- **Audit:** `client.archived`, `client.unarchived`, and `client.deleted`, with
  actions `client.archive`, `client.unarchive`, and `client.delete`. Target type
  `client`, target key the client's integer key. Metadata is `{}`, so no values
  are recorded.
- **Change feed:** actions `archived`, `unarchived`, and `deleted` on
  `client.changed`. The realtime stream sends every feed entry, so open client
  lists refresh for each action.
- **Archived selector:** `archived=active` (default), `archived` (archived
  clients only), or `all`. The cursor binds it with the sort and filters. The
  web list form has the matching select.
- **Omissions:** webhook events `client.archived` and `client.deleted` arrive in
  #307, and client export in #306. MCP and `appctl` archive and delete arrive
  with the agent-surface ticket (#309).

## Addendum: client webhook events (ticket #307, 2026-11-13)

- **Events:** `client.created` on create; `client.updated` on edit, on unarchive,
  and on set or clear of a custom value; `client.archived` on archive;
  `client.deleted` on permanent delete.
  Archive and unarchive of a client already in that state emit nothing. Each
  event is one outbox write (`PublishInTx`) in the change's transaction, so a
  failed write rolls the change back.
- **Payload:** `id` (the client's short ID), `displayName`, `email`, `phone`,
  `notes`, `archivedAt` (null when active), `version`, and `customValues`.
  `customValues` maps field short IDs to stored text. An unset field has no key,
  so absence means unset; the payload never carries `null` values. Values of
  archived fields stay in the payload, because archive keeps values. Delete
  sends the client's last state. `createdAt` and `updatedAt` are not in the
  payload.
- **Not emitted:** define, archive, restore, delete, and option changes on
  fields, and the value clears that field delete performs. Setting or clearing a
  client's custom value emits `client.updated` (see the review fixes addendum).
- **Contract:** `CreateWebhookRequest.events` accepts the four client events
  (`api/openapi.json`, `maxItems` 7). The web form lists them.

## Addendum: final review fixes (spec #294, 2026-11-14)

- **MCP cross-workspace calls (#304):** a Clients MCP tool called with a
  workspace the token is not bound to returns the not-found a missing record
  gets (`client not found`, `custom field not found`, `choice option not found`,
  or `workspace not found` for a collection call). REST answers the same case
  with `404` before the service runs. A role refusal in the token's own
  workspace stays `access is denied`. Items MCP tools keep `access is denied`.
  The authorization matrix pins this per row ([docs/mcp.md](../mcp.md)).
- **Choice sort by label (#301):** a single-choice sort orders by option label,
  empty values last, the same on SQLite and PostgreSQL. A stored value is the
  option's short ID, which SQL cannot decode to the option row, so the sort maps
  the stored ID to its label's rank in a CASE over the field's options, not a
  join. Options with the same label share a rank. The CASE lists at most 100
  options, the slot limit. Number ordering and filtering are unchanged (see
  ADR 0012).
- **Value writes emit events (#307, #308):** setting or clearing a custom value
  emits `client.updated` through the outbox and writes a change-feed row with
  action `updated`, in the same transaction as the value write. A failed write
  of either rolls the value write back. Field definition changes still emit
  nothing.
- **Field lifecycle status:** a field's status is `active`, `archived`, or
  `deleted`. `Status()` takes no `now`, because fields have no time-based
  states. `ValidateFieldTransition` allows archive, restore, and delete from
  archived only. Deleting an active field still returns `ErrFieldNotArchived`.
  Archive, restore, and delete route through it, with the same errors, audit,
  and events. A deleted field has no row, so `deleted` is a transition target
  and never a status that `Status()` returns.
- **Judgement call:** client archive and unarchive keep their `archived_at`
  nullability check. The client lifecycle has no transition validator yet, so
  moving it onto the typed pattern is a separate change.

## Addendum: client list realtime clarification (2026-11-16)

The realtime statements above describe the intended client-list behavior. The
workspace SSE stream and list fragment exist, but the client page does not load
the browser listener or HTMX, so open client lists do not currently refresh from
those events. [#415](https://github.com/dynamiscode/dynamis-code-apps-template/issues/415)
tracks wiring the refresh and protecting unsaved input. Both the stream and
fragment must continue to require `ResourcesRead`; see the
[web realtime contract](../web.md#realtime-contract).
