# 0004: Workspace Webhooks

- Status: accepted
- Date: 2026-08-27
- Owners: template maintainers

## Context

External consumers require delivery of the existing item lifecycle events. The
template has SQLite/PostgreSQL, shared application use cases, audit events,
telemetry, and no broker or generic job system. Webhook secrets must remain
usable for later signing without being stored in plaintext.

## Decision

Add workspace-scoped webhook registration for `item.created`, `item.updated`,
and `item.deleted`. Owners and admins manage registrations with dedicated
`webhooks:manage` and `webhooks:read` permissions. Registration stores the
endpoint URL, selected events, and an AES-GCM encrypted secret; creation and
rotation return the plaintext secret once and never include it in lists,
delivery records, audit metadata, or logs.

Item mutations write matching delivery rows in the same database transaction.
One in-process bounded delivery loop sends Standard Webhooks-style
`Webhook-Id`, timestamp, and HMAC-SHA256 signatures, records redacted status,
and retries a delivery at most five times with exponential delays. Delivery
history is workspace-scoped and retained for one year. Endpoints require HTTPS,
with loopback HTTP allowed for local development; literal and resolved private
addresses are rejected.

## Consequences

Request loss cannot lose a committed matching delivery, and SQLite remains a
single-instance deployment. Delivery is at-least-once, so consumers must
deduplicate by `Webhook-Id`. Cross-replica delivery, operator-triggered replay,
event expansion, and a general job system remain deferred.

## Revisit when

Multiple application instances need shared delivery ownership, delivery volume
exceeds the bounded in-process loop, or consumers require replay and event
contracts beyond the existing item lifecycle.
