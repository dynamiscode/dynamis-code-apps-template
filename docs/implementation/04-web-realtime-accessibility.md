# Phase 04: Web, Realtime, and Accessibility

## Goal

Prove the architecture with baseline server-rendered browser surfaces plus one
useful feature that shares its use case with REST and provides accessible HTMX
and SSE behavior.

## Standards covered

`STANDARDS.md` Sections 2 and 3 web requirements; Section 6 realtime and
accessibility contracts; applicable Section 7 changes; and UI acceptance in
Section 13.

## Prerequisites

Phase 02 identity/workspace policies and Phase 03 HTTP/REST contracts are
complete.

## Required outcomes

- Implement one workspace-owned `item` sample through domain, application,
  repository, web, and REST adapters. An item has an opaque ID, workspace ID,
  creator user ID, non-empty bounded title, completion state, UTC creation and
  update times, and a version for conditional writes.
- Provide create, stable cursor list, get, update title/completion, and explicit
  permanent delete use cases. Define idempotent creation, `If-Match` updates,
  permissions, audit events, and backup-retention consequences.
- Render useful HTML on the server. Use HTMX only for targeted fragments and
  preserve ordinary form/navigation behavior where practical.
- Apply CSRF protection to state-changing browser requests and use the shared
  authorization boundary for every feature operation.
- Publish item-change SSE only for one-way notifications. Authenticate and
  scope every connection; define stable event IDs/types/versions, heartbeat,
  reconnect, missed-event resynchronization, ordering, duplicates, and limits.
- Meet WCAG 2.2 AA fundamentals: semantics, keyboard use, focus, labels,
  errors, contrast, responsive zoom/reflow, and reduced motion.
- Preserve the sample feature until Phase 07 smoke and conformance tests pass.
- Expose baseline browser workflows: workspace creation, member roles/removal
  and ownership transfer, invitation management and acceptance, current-user
  token and session management, configured OIDC login/linking, and workspace
  export. Keep ordinary form fallback and apply CSRF to every mutation.
- Add optional imperative WebMCP registration on eligible pages. Feature-
  detect `document.modelContext`, expose only explicit non-secret schemas,
  prepare visible controls without automatic submission, and prove focus and
  ordinary-browser fallback in browser/template tests.

## Evidence

- Use-case tests prove domain behavior and workspace isolation.
- Web component tests cover full-page and HTMX fragment responses, CSRF,
  authorization, validation, and safe errors.
- SSE tests cover scope, reconnect/resync, heartbeat, limits, and redaction.
- Automated accessibility checks report no unresolved critical or serious
  issues on critical flows.
- WebMCP smoke checks exact tools, schemas, redaction, malformed IDs,
  non-submission, focus, and unsupported-browser fallback.
- A dated manual checklist covers keyboard, focus, forms/errors, zoom/reflow,
  reduced motion, contrast, and one screen-reader pass.
- Full Go test, vet, and race gates pass.

## Exclusions

No SPA runtime, client-side business rules, WebSocket, generic component
framework, visual redesign system, audit administration, account/workspace
deletion, import, backup/restore, maintenance UI, or second sample feature.

## Completion gate

Browser and REST paths reach the same use case, HTMX returns targeted
fragments, SSE has explicit delivery semantics, and accessibility evidence is
linked from the capability ledger.
