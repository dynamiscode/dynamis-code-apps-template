# Navigation and list UX review: #653

**Status: incomplete.** Reviewed `fb53df6` (includes `3a22169`) against baseline `0bb61048` using isolated Playwright contexts, disposable SQLite databases, and synthetic accounts and clients. No application code changed.

## Finding

- **Existing permission dead end:** A workspace member can open Settings → Members and sees an Invitations link. Following it returns 403 Forbidden. Reproduced on both baseline and reviewed revision, so this is not introduced by these commits. The server correctly denies access; the navigation offers a destination the member cannot use.

## Browser evidence

- Owner, desktop 1280×800, light: signed in, opened workspace and clients, created a client, returned to the list, and opened Columns. Baseline and revision screenshots were inspected: list and menu remained usable; reviewed revision adds visible control boundaries. Screenshots contain synthetic data but remain local browser artifacts, not committed or review-accessible.
- Owner, mobile 390×844, dark: opened sidebar; Escape closed it and returned focus to Open navigation. No document-level horizontal overflow in this state. Enter opened Filter.
- Owner, desktop light: created a second similarly named client; Display name sorting produced Alpha then Beta. Enter opened row actions; Open reached the client. Back to clients retained the sort query.
- Owner: workspace-selection header exposed the Workspaces breadcrumb link. Client detail back link worked as above. Deeper breadcrumb traversal was not exercised.
- Member, desktop light: signed in and reached Members. Invitations navigation returned HTTP 403. Same result on baseline.
- Owner, 320×844: sampled visible client-list links; none wrapped, so inline multi-line border behavior remains unverified.

## Checks

`make accessibility-smoke` exited 0. Reported zero axe violations for its tested screens, and passed its keyboard, focus, 320px reflow, reduced-motion, and accessibility-tree checks. This is not a WCAG conformance determination.

## Remaining gaps

- Durable before/after images were not published: browser artifacts excluded from commit. Local images alone do not meet `docs/ux-review.md` evidence-retention requirement.
- Inline links wrapping over multiple lines, deeper breadcrumbs, member mutation controls, admin/viewer roles, error and long-content states, and full list actions remain untested.
- Baseline mobile/dark comparison and full keyboard traversal remain untested. Do not treat this record as a completed UX gate.
