---
name: implement-go-feature
description: Add or change an ordinary Dynamis Code Go vertical feature across shared use cases, web or REST adapters, tests, contracts, and implemented-behavior docs. Do not use for identity, schema-only, MCP-only, or release work.
---

# Implement Go Feature

Use this for recurring product-feature work after the foundation exists. Read
[AGENTS.md](../../../AGENTS.md), the active brief in
[PLAN.md](../../../PLAN.md), and the matching route in
[docs/README.md](../../../docs/README.md).

## Outcome

Deliver one bounded vertical slice whose business behavior lives in a shared
application use case and whose adapters contain transport concerns only.

## Workflow

1. Trace the current feature, callers, authorization policy, transaction
   boundary, repository, adapters, contracts, and tests before editing.
2. Define the workspace scope, permission, validation, transaction,
   concurrency, audit, limit, and failure behavior.
3. Implement domain/application behavior first. Add an interface only at a real
   I/O boundary or useful test seam.
4. Reuse shared HTTP, Problem Details, authorization, audit, telemetry, and
   rendering infrastructure.
5. Update OpenAPI for REST behavior. Keep full-page and HTMX fragment behavior
   consistent; preserve keyboard and non-JavaScript paths where practical.
6. Add the smallest use-case and adapter checks that fail on regression.
7. Update canonical implemented-behavior docs, changelog, and capability
   evidence in the same change.

## Boundaries

- Do not create feature-local middleware, auth, logging, retries, error types,
  or database frameworks.
- Do not add a service, protocol, dependency, or deferred capability without
  its accepted trigger and plan change.
- Route identity changes through `change-go-identity`, database-shape changes
  through `change-go-data`, and MCP/CLI changes through
  `change-go-agent-surfaces`.

## Verify and stop

Run focused tests, then the available verification ladder in `AGENTS.md`.
Stop on a standards conflict, missing workspace boundary, unclear destructive
behavior, or missing canonical contract. Report blockers instead of inventing
policy.
