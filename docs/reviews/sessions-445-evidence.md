# Session list and revocation evidence (#445)

Ticket #445 migrates `/sessions` to the generated templ account layout. The
list identifies the current session and leaves it active while offering
revocation for other active sessions. Revocation remains user-scoped and
CSRF-protected; success feedback and empty state use EN/ES catalogs and
ordinary forms.

- Passed: focused session/account handler tests, the WebMCP browser-surface
  contract test, `internal/i18n` tests, `make lint`, `make templ-check`, and
  `git diff --check`.
- Pending validation window: full `internal/web` tests. The earlier run found
  the missing `/assets/app.js` marker; the targeted contract test passed after
  adding the script.
- Blocked: `make accessibility-smoke`; `axe-core` is absent from `node_modules`.
- Not run: full `make verify` and browser responsive/accessibility smoke.
- No identity policy, schema, API, or changelog changes.

Reconcile this evidence into `docs/web.md` and `docs/capabilities.md` after
related UI issues merge.
