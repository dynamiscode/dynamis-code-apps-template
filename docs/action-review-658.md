# Action guardrail review (#658)

Baseline: integration `455c15b`; `origin/main` was an ancestor at review time.
The source inventory scans all `.templ` files under `internal/web`, including
catalog atoms and feature-gated templates. Four shared shell controls used
`btn-ghost`: theme selection, both mobile drawer triggers, and sign out. They
now use `btn-outline`, retaining their native button semantics and behavior.

`make web-conformance` checks anchor and button opening tags and calls to
`atoms.Anchor`, `atoms.Link`, and `atoms.Button`. It checks literal class
props and rejects conditional-only boundaries, ghost and link variants. It
checks return branches of the two shared dynamic list-class helpers. Exceptions
in `scripts/action-exceptions.json` are path-and-selector scoped; each must
match exactly one control. Regression tests mutate real component calls and
helper returns. This is not a full templ parser: arbitrary component APIs,
generated markup, non-templ HTML, CSS-defined boundaries, and arbitrary dynamic
classes require separate review. Exceptions must be rechecked when their
implementations or callers change.

## Real-app Playwright review

On `da09dd2` plus the guardrail-only changes, a disposable SQLite app and
isolated Playwright contexts used synthetic owner and member accounts. Captures
were inspected and contain synthetic data only.

- Owner, 1280 × 900 light: signed in, opened workspace and Clients; document
  width stayed 1280 px. [Clients](reviews/issue-658/owner-desktop-light.png).
- Owner, 390 × 844 dark: selected Dark, focused the navigation trigger and
  pressed Enter; workspace navigation opened and document width stayed 390 px.
  [Open drawer](reviews/issue-658/owner-mobile-dark.png).
- Member, 390 × 844 light: signed in and opened Clients. Share action was
  absent and direct intake settings returned HTTP 403; document width stayed
  390 px. [Clients](reviews/issue-658/member-mobile-light.png).

## Password reveal follow-up

On `ad51647` with the password reveal fix, a new disposable SQLite server was
verified by its process ID and listening socket before Playwright connected.
At 390 × 844, Spanish login showed a bordered reveal control. Enter changed the
password field to text, `aria-pressed` to true, and its accessible label to
“Ocultar contraseña”; document width remained 390 px. [Spanish login](reviews/issue-658/password-reveal-es-mobile.png).

With JavaScript disabled, owner login succeeded through the ordinary form and
reached Workspaces; no reveal control was rendered. With default OIDC disabled
and no providers configured, login had no OIDC links and an unknown provider
route returned 404. This does not isolate flag behavior from provider setup.
`TestFormUXPatterns` failed before the fix on the transparent reveal border and
passed afterward; it also checks the toggle and accessible state.

## Review gaps

Enabled feature-flag states, other localized journeys, full keyboard traversal,
and other roles and states remain unreviewed. Screenshots do not establish
accessibility conformance. Other JavaScript-generated ghost controls remain
outside the template guardrail and were not changed in this follow-up.
