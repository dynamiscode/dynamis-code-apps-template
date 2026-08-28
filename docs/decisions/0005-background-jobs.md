# 0005: Durable Background Jobs

- Status: accepted
- Date: 2026-08-28
- Owners: template maintainers

## Context

Webhook delivery must survive request disconnects and retry transient failures.
The application is a resource-conscious Go modular monolith with SQLite as its
one-instance default and PostgreSQL required before multiple instances. It has
no broker, cache, or generic asynchronous product contract.

## Decision

Use one database-backed `background_jobs` table and one bounded worker loop per
process. Jobs carry explicit workspace scope, a bounded JSON payload, a
deduplication key, status and attempt timestamps, and a lease token. Claims are
at-least-once; expired leases are reclaimable, handlers are responsible for
idempotency, and handlers receive at most five attempts with exponential
delays. If a lease is exhausted, an optional terminal reconciler settles the
consumer before the job is failed. Failure categories, logs, and metrics are
redacted. Webhook delivery is the first handler and is enqueued in the same
transaction as its outbox row.

## Consequences

Committed webhook work survives request loss and crashed workers without a new
infrastructure dependency. The queue is bounded to one active handler per
process and is not a public long-running-operation API. SQLite remains a
single-instance deployment; PostgreSQL is the required path before multiple
application instances.

## Revisit when

Multiple replicas need shared ownership beyond database leases, queue volume
or handler duration exceeds the bounded process model, or a public product
operation needs progress, cancellation, or scheduling.
