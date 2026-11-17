# Clients list (#415)

`GET /workspaces/{workspaceId}/clients` renders the authorized Clients list with
templ, shared dashboard navigation, and DaisyUI table/card patterns. The page
keeps ordinary GET query forms and links for filters, sorting, saved columns,
and pagination; create and column preferences remain ordinary CSRF-protected
POST forms.

The list and its `/events` stream require `identity.ResourcesRead`. The page
loads local HTMX and `app.js` so the existing `client.changed` and `resync`
events can refresh only `#client-list`, preserving the current path and query.
Refresh defers while a control is focused, a form is dirty, or a form is
submitting. It also defers while a user-opened disclosure is open; query-driven
open disclosures do not block refresh. Closing a user-opened disclosure retries
a deferred refresh after focus leaves. An event that arrives during an in-flight
request schedules one follow-up. A `beforeSwap`
guard checks the live state again immediately before replacing the section and
cancels only realtime GET swaps. Item mutation POST swaps remain owned by
`items.js`. New status nodes restore the actual SSE connection state after each
swap. Events coalesce while a request is in flight; no timer-based retry is
added. Without JavaScript, links and forms retain their server-rendered behavior.

The page uses the workspace locale, theme, and existing responsive table/card
styles. Read-only members see filters and records without create controls.

## Evidence

- `internal/web/clients_test.go`: create permission, query, sorting, filters,
  saved columns, pagination, locale, responsive table/cards, native form
  fallback, and client templ asset wiring.
- `internal/web/clients_events_test.go`: SSE delivery, cross-workspace denial,
  and list fragment rendering.
- `internal/web/assets/realtime-lists.test.mjs`: URL preservation, repeated
  swaps, actual status restoration, delayed-response dirty/focus deferral,
  overlapping refreshes, cancellation ordering with Items row handling, and
  isolation between client and item events.
- `internal/web/assets/forms.test.mjs`: draft representation for multiple
  selected values.
- `scripts/playwright/clients-list.spec.mjs`: browser checks for native
  fallback, a server 422 during peer-tab SSE, multi-select draft restoration,
  query-open disclosure refresh, Spanish labels, responsive light/dark layouts,
  reduced motion, four desktop/mobile screenshot assertions, and an axe WCAG
  2.1 AA scan. Screenshot baselines are pending the queued browser run.
- `go generate ./internal/web`: templ output generation.
- Focused and full verification results are recorded in the PR description.
