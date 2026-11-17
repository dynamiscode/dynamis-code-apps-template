# Issue #422 settings migration evidence

This record covers the general and email settings pages migrated by issue
#422. It supplements [the per-action HTMX review](htmx-437-settings.md) without
editing that shared review or capability evidence.

## Implemented routes

| Route | Behavior preserved | Rendered states |
|---|---|---|
| `GET /workspaces/{workspaceId}/settings/general` | Requires workspace read access. Shows locale controls only to managers and preserves workspace notification preferences. | Current locale, empty notification preference list, and `?saved=1` feedback. |
| `POST /workspaces/{workspaceId}/settings/general` | Requires workspace update permission and session-bound CSRF. Redirects to the general page after save; invalid locale uses the existing management error response. | Existing full-page error and saved redirect behavior. |
| `POST /workspaces/{workspaceId}/settings/notifications` | Requires workspace read permission and session-bound CSRF. Redirects to general settings after save. | Existing full-page error and saved redirect behavior. |
| `GET /workspaces/{workspaceId}/settings/email` | Requires workspace update permission; unavailable email manager remains not found. | Unconfigured and configured states, provider-specific guidance, and saved feedback. |
| `POST /workspaces/{workspaceId}/settings/email` | Requires workspace update permission and session-bound CSRF. The workspace email service verifies the provider before storage. | Redirect on success; 422 alert for invalid settings or provider verification failure; 403 and 500 paths remain unchanged. |

Both pages now use generated templ components, the shared dashboard shell,
shared settings navigation, labelled DaisyUI fields, alert components, and save
feedback. Forms and links remain ordinary HTML requests. HTMX swaps remain
deferred per the accepted review decision.

## Security and accessibility

The existing permission and CSRF checks remain in their handlers. Provider
verification remains in the workspace email service. Passwords, API keys, and
Postmark tokens are not placed in page data or rendered back into form values;
the account token is represented only by its configured status. Settings
responses remain `no-store`. Templ output keeps translated document language,
theme, persistent field labels, error focus targets, and the ordinary form
fallback. Optional navigation uses always-present `Handler` flags, populated
at Files and Clients registration, so profile-pruned builds do not depend on
legacy templates or optional service fields.

## Evidence

- Route behavior: `internal/web/localization.go`, `internal/web/email_settings.go`.
- Templates and shared components: `internal/web/settings_pages.templ`,
  `internal/web/components.templ`, `internal/web/workspace.templ`, and
  `internal/web/navigation.templ`.
- Regression coverage: `internal/web/email_settings_test.go`,
  `internal/web/email_settings_resend_test.go`, and
  `internal/web/email_settings_postmark_test.go`. Generated Core+Identity
  profiles without Files or Clients are covered by
  `TestGenerateWithoutAgentPrunesAgentSurfaceAndBuilds`; Clients without Files
  by `TestGenerateWithClientsProfileRecordsLockAndBuilds`, both in
  `cmd/template-init/main_test.go`.
- English and Spanish empty-state copy: `internal/i18n/locales/en.json` and
  `internal/i18n/locales/es.json`.
- Canonical route/template inventory: `docs/web.md`.
- Browser states, screenshots, axe, keyboard, Spanish, and no-JavaScript checks:
  `scripts/playwright/settings.spec.mjs`.
- Browser cases cover explicit saved light/dark preferences, general settings
  no-JavaScript success, email validation, and member read/permission states
  when member fixture credentials are configured. SMTP, Resend, and Postmark
  verification failures are covered by the focused Go handler/service tests,
  not browser submissions. The browser suite does not exercise a configured
  provider or induce a server failure.

## Checks

On reconciled head `dc5f7fb`, before the Linux baseline adjustment, these passed:
`make workflow-check`, `make templ-check`, `make css-check`, `make doc-links`,
`go test ./internal/web/...` (197 tests), `node --check
scripts/playwright/settings.spec.mjs`, and `git diff --check`. Normal pre-push
`make ci-local` also passed on that head.

CI run `43991899922` on `dc5f7fb` passed Go, PostgreSQL, container,
vulnerability, and workflow/security jobs. The accessibility job failed in
`make browser-smoke`: 29 passed, 3 skipped, and 2 failed. Only the email mobile
light/dark screenshot assertions failed. Both expected 390x1736 and received
390x1808; light differed by 62,726 pixels (9%), dark by 91,007 pixels (13%).
Other settings screenshots, keyboard/axe, Spanish, and no-JavaScript settings
checks passed. The member permission case skipped without fixture credentials;
native member permission coverage remains unverified.

The reviewed actuals show the same page content and controls, with one extra
wrapped line in each of three email paragraphs, adding 72px total. The exact
resolved CI font was not recorded. The failure artifact contained only four
email mobile actual/diff PNGs; actuals show synthetic `UI Baselines` and blank
provider fields. The new Linux-only email mobile baselines are byte-identical
copies of those reviewed actuals; all other snapshot names remain unchanged.
The baseline adjustment has not yet run in CI.

A prior Linux MCR run on `ee96147` passed 25 tests with one skip but did not
reproduce this GitHub mismatch. A local `make accessibility-smoke` passed in
English and Spanish on that earlier head. The mismatch remains unresolved
until the updated head passes browser CI. Provider verification failures have
focused Go coverage; the browser suite does not submit to a configured provider
or induce a server failure.
