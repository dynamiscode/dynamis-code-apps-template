# 0017: Immutable workspace transactions entries

- Status: accepted
- Date: 2026-11-26
- Owners: template maintainers

## Context

Small businesses need a workspace-scoped record of money in and out, including
cash, without silently overwriting history or mixing currencies. This decision
covers the first entry slice of #570; later corrections and reporting need their
own implementation and evidence.

## Decision

Transactions is optional and requires Identity and Clients. Each entry belongs to one
workspace and records direction, a positive integer amount in minor units,
currency, payment method, occurred-on date, optional same-workspace client and
note, creator, and creation time. An omitted occurred-on date defaults to today
in the workspace time zone; backdated entries are allowed. Check, transfer,
cash, and card are the initial methods. Cash follows the same entry rules as
other methods.

Entries are immutable: this slice exposes creation and paginated listing, not editing or
deletion. Lists use pages of at most 100, numbered 1 through 1,000,000; MCP and
CLI accept a page argument. Creation is audited. Owners, admins, and members can create and read
entries; viewers and outsiders cannot. Authorization and workspace isolation
are enforced in the shared service, including calls from web, REST, MCP, and
the REST-only CLI. Members cannot access totals.
Permanent client deletion clears the client reference while preserving the
money entry and its other fields.
The web form offers a searchable, paginated client selector. Entry lists show
client and current member names, and a former-member label when the author is
no longer a workspace member. Successful creation displays confirmation at the
entry list; invalid currency has an associated field error.

## Consequences

Integer amounts avoid floating-point rounding and retain the original currency;
no conversion or cross-currency total is implied. Owners and admins may void
entries, retaining their details, actor, time, and optional reason. A new entry
may reference a voided entry as its replacement. Staff can record refunds
against a same-workspace, unvoided receipt, capped at its remaining amount in
the same currency. A receipt cannot be voided while it has active refunds.
Voided refunds no longer consume the remainder; replacement refunds retain the
original receipt link. Method management, categories, and reporting are separate.
Engagement and Catalog integration, expected payments, and tax reporting remain
outside this slice.

## Revisit when

A correction, refund, method management, or reporting slice is accepted, or a
new integration changes the entry model or authorization boundary.
