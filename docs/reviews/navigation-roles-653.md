# Navigation and list UX review: #653

**Status: incomplete.** Reviewed `fb53df6` (includes `3a22169`) against baseline `0bb61048` using isolated Playwright contexts, disposable SQLite databases, and synthetic accounts and clients. No application code changed.

## Finding

- **Existing permission dead end:** A workspace member can open Settings → Members and sees an Invitations link. Following it returns 403 Forbidden. Reproduced on both baseline and reviewed revision, so this is not introduced by these commits. The server correctly denies access; the navigation offers a destination the member cannot use.

## Browser evidence

- Owner, desktop 1280×800, light: signed in, opened workspace and clients, created a client, returned to the list, and opened Columns. [Baseline](navigation-roles-653/before-desktop.png) and [revision](navigation-roles-653/after-desktop.png) captures were inspected. Both files have identical SHA-256 hashes, so they do not demonstrate a visual change; the earlier claim that this comparison showed added boundaries was unsupported. The captured list and menu were usable.
- Owner, mobile 390×844, dark: [opened sidebar](navigation-roles-653/after-mobile-dark.png); Escape closed it and returned focus to Open navigation. No document-level horizontal overflow in this state. Enter opened Filter.
- Owner, desktop light: created a second similarly named client; Display name sorting produced Alpha then Beta. Enter opened row actions; Open reached the client. Back to clients retained the sort query.
- Owner: workspace-selection header exposed the Workspaces breadcrumb link. Client detail back link worked as above. Deeper breadcrumb traversal was not exercised.
- Member, desktop light: signed in and reached Members. Invitations navigation returned HTTP 403. Same result on baseline.
- Owner, 320×844: sampled visible client-list links and dashboard inline links; none wrapped, so inline multi-line border behavior remains unverified. Source inspection shows the shared home breadcrumb component produces at most one Workspaces item; no deeper breadcrumb journey was identified.

## Checks

`make accessibility-smoke` exited 0. Reported zero axe violations for its tested screens, and passed its keyboard, focus, 320px reflow, reduced-motion, and accessibility-tree checks. This is not a WCAG conformance determination.

## Remaining gaps

- Committed screenshots contain only synthetic workspace and client names; no credentials, session values, or personal data were visible on inspection. The identical desktop files leave the visual before/after comparison inconclusive; recapture independently before asserting a visual difference.
- Inline links wrapping over multiple lines, member mutation controls, admin/viewer roles, error and long-content states, and full list actions remain untested.
- Baseline mobile/dark comparison and full keyboard traversal remain untested. Do not treat this record as a completed UX gate.
