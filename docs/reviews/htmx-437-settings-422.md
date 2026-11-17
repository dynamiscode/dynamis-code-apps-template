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

Passed: `make templ-check`; four focused web route/provider/form-markup tests;
`go test ./cmd/template-init -run '^(TestGenerateWithoutAgentPrunesAgentSurfaceAndBuilds|TestGenerateWithClientsProfileRecordsLockAndBuilds)$' -count=1`;
`go test ./internal/i18n -count=1` (4 tests); `node --check
scripts/playwright/settings.spec.mjs`; and `make doc-links`.

`make css-check` is blocked: `npm exec -- tailwindcss` could not resolve an
executable in this worktree. The Playwright checks and remaining verification
ladder are pending the serialized test window; do not treat the new browser
coverage as executed yet.

Browser coverage for responsive layouts, keyboard navigation, reduced motion,
explicit light/dark preferences, Spanish, and no-JavaScript success/validation
remains subject to the browser checks reported with this change. This is not a
full state matrix: provider verification failure is covered by the focused Go
handler/service tests; the browser suite does not exercise a provider timeout
or server failure. Member browser permission coverage skips without fixture
credentials. Page-render failures remain unverified by a named email settings
test.
