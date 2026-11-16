# Session list and revocation evidence (#445)

Ticket #445 migrates `/sessions` to the generated templ account layout. The
list identifies the current session, distinguishes active, expired, and revoked
sessions, and offers revocation for every active session, including current.
Revocation remains user-scoped and CSRF-protected; revoking current session
redirects to sign-in on the next request. Feedback and empty states use EN/ES
catalogs and ordinary forms.

- Passed: focused session handler tests for self and cross-user revocation,
  CSRF, expired status, and Spanish empty state; WebMCP browser-surface contract;
  `make lint`, `make templ-check`, and `git diff --check`.
- Pending validation window: full `internal/web` tests. The earlier run found
  the missing `/assets/app.js` marker; the targeted contract test passed after
  adding the script.
- Blocked: `make accessibility-smoke`; `axe-core` is absent from `node_modules`.
- Not run: full `make verify` and browser responsive/accessibility smoke.
- No identity policy, schema, API, or changelog changes.
