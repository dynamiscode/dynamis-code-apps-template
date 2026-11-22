---
name: alpinejs-app
description: How this repo writes browser JavaScript - the vendored @alpinejs/csp build, every behavior registered as an Alpine.data component in internal/web/assets/alpine-components.js, pure evaluators exported for node tests, the data-messages catalog, and the strict CSP no-inline rule. Use when adding or changing any client-side behavior, mount point, or evaluator, or when wiring a new page to pageBehaviors.
---

# Alpine.js app behaviors

## The architecture

One file: `internal/web/assets/alpine-components.js`. The vendored
runtime is the official `@alpinejs/csp` build
(`internal/web/assets/js/alpine.min.js`, exception record
`docs/decisions/exceptions/2026-11-20-alpinejs-browser-runtime.md`) -
chosen because the CSP forbids `unsafe-eval`, so NO inline expressions
anywhere: no `x-on:click="count++"`, no `:class="open"`. Every behavior
is a named component registered once:

```
document.addEventListener("alpine:init", () => {
  window.Alpine.data("pageBehaviors", pageBehaviors); // etc.
});
```

## Mount points

- `x-data="pageBehaviors"` on the three shells' `<body>`
  (`templates/workspace.templ`, `auth.templ`, `public_pages.templ`) -
  wires `wireForms`, `wireRealtime`, `wireFileUploader`, `wireWebMCP`,
  and `toggleDrawer`.
- `x-data="listScreen"` on list
  screens; `x-data="notificationsLive"` on `.notification-center`;
  `x-data="passkeyLogin"` / `passkeyEnroll` on auth/security sections.

Adding behavior to a page = a method on the mounted component or a new
component registered + mounted, never a script tag, never an inline
expression.

## The rules

1. **Components hold state and DOM; evaluators decide.** Business rules
   go in exported pure functions (`core`,
   `restoreFailedOptimisticRow`, the FileUploader state machine
   helpers): plain `(input) -> output` with no DOM access.
2. **Every evaluator gets node tests.**
   `internal/web/assets/*.test.mjs` run by `make test-unit`
   (38/38 today). A new evaluator without its table/exhaustive test is
   unfinished; a DOM-wiring change without one runnable check is too.
3. **Text comes from the catalog.** All user-facing strings are
   `msg("key")` reads from the src-less
   `<script data-messages data-msg-*>` block in `UiHead` - add the key
   to `internal/i18n/locales/en.json` AND `es.json` in the same change
   (both files must stay valid JSON - a double comma broke the build
   once). Server-rendered text keeps using `data.T(...)`.
4. **HTMX loads after Alpine** (document order). Code needing
   `window.htmx` (wireRealtime) retries until it exists - keep
   that guard when wiring new realtime behavior.
5. **JS enhances, never gates.** The server path must work without
   JavaScript: forms POST, validation errors render, links navigate
   (repo invariant; the browser suite drives key flows with JS off).
6. **Marker classes stay.** JS selects `.toasts`, `.toast`,
   `form.client-query`, `[data-upload-progress]`, `.list-search` - grep
   `alpine-components.js` for a class before deleting it from templ.

## Checks

- `make js-check` (syntax), `make test-unit` (evaluator tables).
- Change touching wireForms/wireRealtime/FileUploader has deeper
  contracts: load the `form-drafts`, `async-feedback`,
  `file-upload-form`, `form-field-input`, or `form-wizard` skill that
  matches the mechanism and run its checks.
- Browser proof: `go test -tags browser -count=1 ./internal/browsertest/
  ... -run '<names>'` - the suite drives the real registration, CSP and
  all.
