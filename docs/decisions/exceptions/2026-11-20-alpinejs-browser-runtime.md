# Exception: no SPA runtime — Alpine.js adoption

- Profile and requirement: template conformance, architecture invariant "Do not
  add an ORM, DI container, broker, cache, SPA runtime, protocol, or service
  until its trigger in `docs/capabilities.md` is accepted."
- Status: accepted
- Responsible owner: repository maintainer
- Approving maintainer: repository maintainer
- Security-owner approval, when required: n/a (no auth/session/token change)
- Created: 2026-11-20
- Review by: 2027-02-20
- Expires: 2027-02-20 (or on removal of Alpine)

## Reason and evidence

The user explicitly approved replacing the app's hand-written browser JS
(`forms.js`, `app.js`, `list-table.js`, `items.js`, `mfa.js`,
`notifications.js`, `upload-states.js`) with Alpine.js as part of the DaisyUI
adoption on branch `daisyui-templates`. Two atoms (`pricing.templ`,
`range.templ`) already reference Alpine; serving one vendored file makes them
usable instead of dead markup. Server-rendered HTML + htmx + SSE remain the
architecture; Alpine is a progressive-enhancement layer, not a SPA runtime in
practice, but it is a new browser runtime and therefore recorded here.

## Scope note (close of the adoption pass)

Alpine is the browser runtime for the whole application. Every former
app script (forms.js, app.js, list-table.js, items.js, mfa.js,
notifications.js) now lives in `assets/alpine-components.js` as
registered `Alpine.data()` components mounted from markup, running on
the CSP build; the logic moved as-is, so the exhaustive node, Go replay,
and browser coverage still applies. Removing Alpine later means moving
those wire functions back into plain deferred scripts.

## User, security, compatibility, and operational risk

- User: markup behavior changes must keep ids, names, `data-*` hooks, aria,
  and CSRF semantics; browser smoke tests gate each port.
- Security: Alpine evaluates `x-*` attributes server templates emit; only
  trusted server-rendered templates carry them. No user input is ever
  interpolated into Alpine expressions.
- Compatibility: 45 KB vendored `alpine.min.js`, no CDN dependency, served
  from the existing embedded `/assets/` file server.
- Operational: one extra deferred script tag on pages that use Alpine.
- Security: the app CSP is `script-src 'self'` without `unsafe-eval`, so the
  vendored runtime is the official `@alpinejs/csp` build (same version), which
  parses restricted expressions instead of using the Function constructor.
  All app logic lives in registered `Alpine.data()` components, never in
  inline attribute expressions.

## Compensating controls

- Alpine loaded `defer` only; pages without `x-*` attrs pay one unused request.
- All behavioral contracts (ids, names, data-*, aria, CSRF) preserved and
  covered by `make test-unit`, `make browser-smoke`, `make accessibility-smoke`.

## Replacement or removal path

If Alpine is later rejected, drop the `x-*` attrs and the script tag; each
Alpine component maps back to a small vanilla handler or an htmx swap.

## Verification

- `make test`, `make lint` green after adoption.
- `make browser-smoke` and `make accessibility-smoke` run after each JS port.
