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
- Passed: `PLAYWRIGHT_PORT=58101 ./scripts/playwright-smoke.sh scripts/playwright/webhooks.spec.mjs --update-snapshots` (2 tests), the same command without snapshot updates (2 tests), and the full browser suite after the fixture fix (16 tests). The fix makes `openWorkspace` select the seeded `UI Baselines` workspace by accessible name; the full suite creates a Spanish workspace immediately before the webhook tests. Before this fix, CI run 43974340103 failed both webhook cases because they expected the English `Name` label on the first workspace card. The updated full suite passed in the same order.
- Passed: `make accessibility-smoke` rerun after the earlier `StaleElementReferenceError` at `scripts/accessibility.mjs:198`; both EN/ES routes had zero axe violations, with keyboard, focus, 320px reflow, reduced motion, and accessibility tree checks passing. The stale-element error did not reproduce on rerun.
- Passed: `make test`, `make lint`, the normal commit and pre-push hooks (including race Go suite, API/templ/CSS drift, module verification, build, and gitleaks), and CI `make verify` plus PostgreSQL and other checks. The initial CI run had 9/10 checks pass; browser-smoke failure is fixed locally but CI has not rerun on the follow-up commit.
- Pending: commit and push the workspace-selection fix through the next serialized hook/stack window, then verify new CI. No changes to shared navigation, common JS, delivery runtime, or database schema.
- Rebased onto verified `origin/main` `11cf5bc5aa959e056b63a145c30f921ab2127f98`, preserving #445 session route docs, render dispatch, localization keys, capability evidence, and generated session templates. Tailwind and generated CSS checks passed on this base; check again if upstream advances before final validation.

`docs/web.md` now records the templ renderer and required delete confirmation. Shared `docs/capabilities.md` reconciliation remains deferred to coordinated #374 integration.
