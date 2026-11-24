# Navigation and list UX review: #653

**Status: incomplete.** Reviewed `fb53df6` (includes `3a22169`) against baseline `0bb61048` using isolated Playwright contexts, disposable SQLite databases, and synthetic accounts and clients. No application code changed.

## Finding

- **Existing permission dead end:** A workspace member can open Settings → Members and sees an Invitations link. Following it returns 403 Forbidden. Reproduced on both baseline and reviewed revision, so this is not introduced by these commits. The server correctly denies access; the navigation offers a destination the member cannot use.

## Browser evidence

**Correct-revision real-app follow-up:** Built `cmd/server` from worktree revision `c55a1148685822fd3f1026027b7bd4916cc2fad6`, with a disposable SQLite database and synthetic owner and clients. Playwright loaded `/assets/components.css` and `/assets/app.css`; their served SHA-256 hashes were respectively `29ac5558317ab695add60729df836b78fc1afde456b9cc41611c40c6d8de7adc` and `6909e1fe30aaf930e580492fff4d6e730f538797502e115a4fbe202f0b7f8a8b`, matching the target row below, not baseline. The home header `.workspace-name` computed `border-top-width: 1px`.

- **Populated desktop list:** Created Synthetic Alpha and Synthetic Beta through the real forms. Sorting Display name returned Alpha before Beta; keyboard Enter opened Columns and showed its controls. Enter also opened the first row actions, exposing Open, Edit and Delete permanently. [Columns and sorted list](navigation-roles-653/real-target-list-columns.png).
- **Mobile dark:** At 390×844, system dark preference rendered a dark background; the populated list displayed both clients, with document width equal to viewport width. Keyboard Enter opened Filter. [Mobile list](navigation-roles-653/real-target-mobile-dark.png). At 320×844, navigation opened and its links were visible. At 390×844, Escape left focus on Open navigation after a keyboard activation; visibility immediately after that activation was not established. [Open navigation](navigation-roles-653/real-target-mobile-navigation.png).
- **Wrapped inline link:** At an intentionally extreme 200×844 viewport, the real dashboard link “View all — Due today” wrapped into two client rectangles and computed `box-decoration-break: clone`. [Wrapped link](navigation-roles-653/real-target-inline-wrapped.png). This demonstrates the wrapped state, not 320px reflow compliance.

Screenshots were inspected and show synthetic names only; no credentials or session values. These checks do not cover every role or list action.

**Independent preview comparison:** Separate baseline `0bb61048` and target `fb53df6` preview binaries, populated synthetic workspace selection, isolated Playwright contexts. Inspected [baseline desktop](navigation-roles-653/preview-before-desktop.png) and [target desktop](navigation-roles-653/preview-after-desktop.png) at 1280×800 light, and [baseline mobile](navigation-roles-653/preview-before-mobile-dark.png) and [target mobile](navigation-roles-653/preview-after-mobile-dark.png) at 390×844 dark. Header brand link gains a border: computed `border-top-width` is `0px` baseline and `1px` target at both sizes. Captures contain synthetic workspace names and no credentials, session values or personal data. Preview does not test authorization or production data.

Screenshot SHA-256: desktop baseline `3ddd84c3d9f8f568b0d15f5ce2a223b5220e8c51c6446c55131c26f92551325b`, target `f2706a44b8b02f95a6acf8e3c757de8d0e13992af65b9e30e8a4de257076bd1f`; mobile baseline `98310c90d1376864fca0f3e6002f72ac57ba44fd4c318c91bce2b348f5d0ef5c`, target `70db802a015e1684a81ddd68eb6eb3408051a7b5e1f8c61501d2e625f3640880`. Both pairs differ.

**Correction:** The original after server was accidentally built from the invoking worktree, not `navigation-roles-653`: its served CSS hashes match baseline. Therefore the original list, mobile, keyboard and breadcrumb observations below are baseline observations, not proof of the reviewed revision. The member permission dead end was independently repeated against the correctly built revision.

Fresh owner home captures, desktop 1280×800 light, synthetic data: [before](navigation-roles-653/verified-before-home.png) at `http://127.0.0.1:18954/workspaces/njuz`, revision `0bb61048`; [after](navigation-roles-653/verified-after-home.png) at `http://127.0.0.1:18953/workspaces/njuz`, revision `fb53df6`. Playwright read computed `.workspace-name` border width: before `0px`, after `1px`. Served asset SHA-256:

| Revision | `components.css` | `app.css` |
|---|---|---|
| `0bb61048` | `af0896bbd383989071f2c872998b881672a30369c5dec017989e09f285927d20` | `3f4642fe43ef875c27d6e4aa180254615c1ca4c8461d159bbdaec41ebb5344ab` |
| `fb53df6` | `29ac5558317ab695add60729df836b78fc1afde456b9cc41611c40c6d8de7adc` | `6909e1fe30aaf930e580492fff4d6e730f538797502e115a4fbe202f0b7f8a8b` |

On the earlier correctly built revision at 320×844, both dashboard inline links computed `box-decoration-break: clone` but each occupied one line. The follow-up above exercises wrapping at 200px.

Original observations, retained as baseline-only:

- Owner, desktop 1280×800, light: signed in, opened workspace and clients, created a client, returned to the list, and opened Columns. [Baseline](navigation-roles-653/before-desktop.png) and [revision](navigation-roles-653/after-desktop.png) captures were inspected. Both files have identical SHA-256 hashes, so they do not demonstrate a visual change; the earlier claim that this comparison showed added boundaries was unsupported. The captured list and menu were usable.
- Owner, mobile 390×844, dark: [opened sidebar](navigation-roles-653/after-mobile-dark.png); Escape closed it and returned focus to Open navigation. No document-level horizontal overflow in this state. Enter opened Filter.
- Owner, desktop light: created a second similarly named client; Display name sorting produced Alpha then Beta. Enter opened row actions; Open reached the client. Back to clients retained the sort query.
- Owner: workspace-selection header exposed the Workspaces breadcrumb link. Client detail back link worked as above. Deeper breadcrumb traversal was not exercised.
- Member, desktop light: signed in and reached Members. Invitations navigation returned HTTP 403. Same result on baseline.
- Owner, 320×844: sampled visible client-list links and dashboard inline links; none wrapped, so inline multi-line border behavior remains unverified. Source inspection shows the shared home breadcrumb component produces at most one Workspaces item; no deeper breadcrumb journey was identified.

## Checks

`make accessibility-smoke` exited 0. Reported zero axe violations for its tested screens, and passed its keyboard, focus, 320px reflow, reduced-motion, and accessibility-tree checks. This is not a WCAG conformance determination.

## Remaining gaps

- Screenshots contain only synthetic workspace and client names; no credentials, session values, or personal data were visible on inspection. Fresh home captures prove the sampled header change; the original identical list images are baseline-only and do not prove list changes.
- Member mutation controls, admin/viewer roles, error and long-content states, full list actions, and full keyboard traversal remain untested. Do not treat this record as a completed UX gate.
