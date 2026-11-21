---
name: htmx-patterns
description: How this repo uses htmx - the vendored runtime, targeted swaps for mutations, SSE for one-way realtime delivery, no WebSockets, server-first fallback, and the event names Alpine listens for. Use when adding or changing any hx-* attribute, SSE stream, swap target, or realtime list refresh.
---

# htmx patterns

## The rules

1. **Server-rendered first.** Every htmx attribute decorates a flow
   that works without it: the form POSTs, the server re-renders, the
   link navigates. htmx only makes the round trip targeted
   (`hx-post` + `hx-target` + `hx-swap`) - it never carries logic.
2. **One-way realtime is SSE.** Live lists subscribe to
   `data-events={ path + "/events" }` (`#item-list`, client lists) and
   the server streams `htmx-sse` messages; the refresh is applied by
   Alpine's `wireRealtime` (`app:refresh`-style events), which waits
   while a row is being edited and preserves dirty state. No
   WebSockets: only measured bidirectional need would trigger one
   (`docs/capabilities.md` governs that).
3. **Idempotent, guarded mutations.** Create forms carry
   `idempotency_key`; double presses are also stopped client-side
   (`hx-sync`, busy state, see `item-create-submission` skill).
   Responses use `HX-Trigger` for feedback (`app:toast`, ASCII-escaped
   by `asciiJSON`).
4. **Swap focus contract.** After a swap, focus returns to the same
   field or to the new error - the `htmx:afterSwap` handlers in
   `wireForms` own it; do not write a second handler, extend that one.
5. **Vendored runtime.** `internal/web/assets/htmx.min.js` (license
   `HTMX-LICENSE`); no CDN. It loads after Alpine; guards that retry
   until `window.htmx` exists stay.
6. **Toast channel.** Save/failure feedback goes through the
   `.toasts` region (role=status, dismissible, 6s) - one channel for
   the whole app, never per-page alerts for async results.

## Event names Alpine listens for

`htmx:afterSwap` (focus restore), `htmx:configRequest` (headers), SSE
message events on the subscribed element, and the page's own
`app:*` triggers. Add new triggers at the server with
`w.Header().Set("HX-Trigger", ...)` and listen in one place
(`wireRealtime`/`wireForms`), not per page.

## Debugging

The `htmx-debugging` skill is the diagnostic path when a request does
not fire or a swap does not land: check the request in the network tab,
the `hx-*` inheritance, and the swap target first.

## Checks

- Server route still works without JS: curl the POST with csrf and read
  the redirected HTML (the browser suite does this in
  `TestFilesKeyboardAndSpanishLabels`-style flows and JS-off journeys).
- Realtime changes: `go test -tags browser -count=1
  ./internal/browsertest/... -run 'TestClientsListQueryOpenDoesNotBlockRealtime|TestItems'`
  and the evaluator tables in `make test-unit`.
- SSE contract details live in `docs/web.md` "Realtime contract" -
  read it before changing event names.
