# Action guardrail review (#658)

Baseline: integration `455c15b`; `origin/main` was an ancestor at review time.
The source inventory scans all `.templ` files under `internal/web`, including
catalog atoms and feature-gated templates. Four shared shell controls used
`btn-ghost`: theme selection, both mobile drawer triggers, and sign out. They
now use `btn-outline`, retaining their native button semantics and behavior.

`make web-conformance` checks literal anchor and button opening tags. Its
path-and-selector exceptions are documented in `scripts/action-exceptions.json`;
each selector must match exactly one control. The check rejects unbounded
literal controls and ghost/link button variants. It is not a full templ parser:
component call-site props, generated markup, non-templ HTML, CSS-defined
boundaries, and arbitrary dynamic classes require separate review. Shared
component and catalog exceptions must be rechecked when their implementations
or callers change. It does not certify all action roles or detect all possible
drift.

## Review gaps

No real-app browser review or screenshots were collected for this change.
Light/dark, mobile/desktop, keyboard, localization, permissions, feature flags,
and no-JavaScript journeys remain unverified at this revision. Earlier reviews
under `docs/reviews/` and `docs/action-review-657.md` are not evidence for these
changes. Rendered UX review remains blocked on completing those journeys and
retaining sanitized evidence, as required by `docs/ux-review.md`.
