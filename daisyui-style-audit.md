# daisyUI/Tailwind style replacement audit (temporary tracker)

Every custom CSS rule in app.css / clients.css / import.css is a finding.
Plan: replace with a daisyUI component class when one exists, else Tailwind
utilities on the element; delete the replaced CSS rule. Status marked when done.

| # | Finding | Files | Replacement | Status |
|---|---|---|---|---|
| W1a | `.topbar` shell + `.topbar-context` + `.topbar-title` | app.css, workspace.templ, dashboard_header.templ | daisyUI `navbar` + utilities | done |
| W1b | `.app-shell` grid + `.sidebar` + `.sidebar-back` + max-38rem sidebar block | app.css, templates/workspace.templ, home_navigation.templ | daisyUI `drawer drawer-open lg:drawer-open` | done |
| W1c | `.workspace-main` width rules | app.css, pages | Tailwind `max-w-*` on main | done |
| W2a | `.cards a`, `.items` li, `.workspace-cards` | app.css, workspace page | daisyUI `card` | done |
| W2b | `.first-task`, `.notification-empty`, `.auth-card`/`.narrow` | app.css | daisyUI `card` + border-s utilities | done |
| W2c | `.webhook-card`, `.session-card`, `.notification-card`, `.settings-panel`, `.settings-page section/form`, `.settings-record`, `.client-panel`, `.client-value`, `.client-field-card`, `.client-field-option-list li`, `.file-upload-panel`, `.list-view-cards li`, `#item-list .items li` | app.css, clients.css | daisyUI `card` | done |
| W2d | `import.css` preview/error/confirm | import.css | daisyUI `card` + utilities | done (import.css deleted: rules dead, file never linked; preview section now daisyUI card) |
| W3 | `.list-badge`, `.session-status*`, `.notification-read/unread`, `.list-view-badge`, `.list-view-summary-current`, `.client-bulk-count` | app.css | daisyUI `badge` (+ variants) | done |
| W4a | `.toast`/`.toasts` (JS-created) | app.css, alpine-components.js | daisyUI `toast toast-end toast-bottom` + `alert` | done |
| W4b | `.error-summary`, `.field-error`, `.draft-notice`, `.strength` (JS-created) | app.css, alpine-components.js | daisyUI `alert alert-error` / `label` / `progress`? evaluate | done (fieldset note-reservation rules kept: a11y, see form-field-input skill) |
| W4c | form[aria-busy] CSS spinner + @keyframes spin | app.css, alpine-components.js | daisyUI `loading loading-spinner` span | done (setBusy injects daisyUI loading-spinner; CSS spinner + keyframes deleted) |
| W4d | `.reveal`, `.secondary`, `.wizard-nav/status` | app.css, alpine-components.js | daisyUI `btn btn-ghost btn-xs` / utilities | done |
| W5a | global `input, select, button` element styling | app.css | delete; daisyUI inputs/buttons already classed | done |
| W5b | global `label` styling | app.css | delete; label classes at use sites | done |
| W5c | global `table/th/td` styling | app.css | delete; daisyUI `.table` | done |
| W5d | `.visually-hidden` → Tailwind `sr-only` | app.css, 11 templ uses | class swap, delete rule | done |
| W5e | `.skip-link` | app.css, skip_link.templ | Tailwind `sr-only focus:not-sr-only` pattern | kept (skip-link CSS retained deliberately: focus-positioning, a11y-critical) |
| W6 | clients.css remainder (match/field-choices/steps/detail dl) | clients.css | Tailwind utilities inline | done (chips/filter cosmetics -> utilities; structural dl/steps kept) |
| W7 | residual layout rules (grids, spacing, sticky columns) | app.css | Tailwind utilities inline where touched | done (daisyUI-conflicting overrides + dead .error/.warning/.success/.secondary deleted; structural layout kept) |
| W8 | final: rebuild CSS, make test, browser-smoke, a11y smoke | — | verify | done: full browser suite 0 failures, accessibility-smoke EXIT 0, make verify green, templ 0 drift |

Rules kept on purpose: tokens.css (design token layer), `:focus-visible` global
(a11y), prefers-reduced-motion globals, JS-hook rules with no class equivalent
([aria-busy], [data-optimistic], data-submit-button).
