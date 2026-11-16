# Webhook migration evidence (#424)

Issue #424 migrates the workspace webhook registry and delivery history to generated templ using the existing dashboard navigation and DaisyUI-backed components. The server-rendered views preserve their ordinary links and CSRF-protected forms.

## Route and safeguard evidence

- `GET /workspaces/{workspaceId}/settings/webhooks` keeps the shared `WebhooksRead` authorization, selected locale and theme, and `Cache-Control: no-store` response.
- Create, rotate, and delete remain ordinary POST forms. Delete requires a localized confirmation checkbox, and the handler rejects missing confirmation before calling the service. The existing handler checks `WebhooksManage` and session-bound CSRF. Creation and rotation secrets remain limited to the immediate no-store response.
- Missing secret encryption configuration displays a localized warning and disables create and rotate controls. Server-side validation remains authoritative.
- Delivery history uses the existing `WebhooksRead` policy and workspace-scoped service query. The view renders event type, status, attempts, HTTP status, timestamps, and `LastError`; it never receives or renders delivery payload or webhook secret. HTML is escaped by templ.
- Existing tests cover forbidden workspace access and missing CSRF, one-time secret behavior, secret readback failure, unavailable secret key, create/rotate/delete, delivery redaction, and deleted or unknown history. New assertions verify the exact registration ID remains after an unconfirmed delete, localized English/Spanish confirmation copy, DaisyUI table/card output, theme metadata, and no-store history.
- Empty list and empty history use translated empty-state copy. Validation and failure messages remain rendered on the registry page through the existing handler behavior.

## Verification

`scripts/playwright/webhooks.spec.mjs` adds real-browser coverage for one-time secret display and list redaction, delivery history and payload redaction, no-store responses, four desktop/mobile light/dark screenshots, mobile overflow, keyboard skip-link and form navigation, and axe checks on secret and history states. Its JavaScript-disabled case covers the empty-history link and required delete checkbox fallback. The local smoke server supplies an isolated fixture key; the delivery receiver binds loopback only. The route test covers Spanish copy and server rejection of a forged unconfirmed delete.

- Passed: focused webhook lifecycle/secret Go tests, `node --check scripts/playwright/webhooks.spec.mjs`, Playwright test discovery, `make templ-check`, `make css-check`, and `git diff --check`.
- Passed: issue #424 skill review on spec and standards axes. No remaining review findings. The standards review requested verification that CSS drift checks include generated component CSS; `make css-check` regenerates from the scoped Tailwind source and compares the generated output.
- Passed: `PLAYWRIGHT_PORT=58101 ./scripts/playwright-smoke.sh scripts/playwright/webhooks.spec.mjs --update-snapshots` (2 tests), same command without snapshot updates (2 tests), and `make accessibility-smoke` (English and Spanish routes, zero axe violations; keyboard, focus, 320px reflow, reduced motion, and accessibility tree checks passed). Generated and visually reviewed desktop/mobile light/dark baselines.
- Still queued: full-validation window and serialized `gh stack` submission. `make test`, `make lint`, race, generated API drift, PostgreSQL, docker smoke, and `make verify` were not run in this focused window.
- `origin/main` advanced to `11cf5bc5aa959e056b63a145c30f921ab2127f98` after #445 and #468. Rebase onto that base before final checks, preserving its session route docs, session render dispatch, localization keys, capability evidence, and generated session templates while retaining webhook changes. Recheck Tailwind and generated CSS against then-current upstream for #429/#429 follow-on changes.

`docs/web.md` now records the templ renderer and required delete confirmation. Shared `docs/capabilities.md` reconciliation remains deferred to coordinated #374 integration.
