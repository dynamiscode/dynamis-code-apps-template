# Engagement workflow decisions (#621)

These are approved requirements for the redesign in #619, not claims about
current behavior. Existing behavior remains documented in [capabilities](capabilities.md),
[API](api.md), [web](web.md), and [data lifecycle](data-lifecycle.md).

## Approved

- Existing engagements receive an editable name generated from a snapshot of
  the client name followed by ` — ` and the engagement short ID. The name is
  required, including in the existing REST API. REST v1 create also requires
  `stageId`, as explicitly approved by the user. A later client rename does not
  change the snapshot.
- Date boundaries use UTC.
- An engagement has at most one active action. Completion, replacement, and
  clearing retain immutable action history visible to readers. Existing writers
  manage active actions and may reopen completed actions. History retains action
  text, due date, owner, outcome, actor, and time until engagement deletion.
  Member removal or account deletion clears the affected identity reference,
  preserving the remaining history.
- Archived stages remain visible, including their occupants; moves out are
  allowed but moves into archived stages are not. Dashboard counts include work
  in open stages, including archived-stage occupants.
- Deletions cascade. Workspace export includes stages, engagements, active
  actions, action history, stage-change history, and available actor identities.
- Upcoming, undated, and completed queue filters include engagements in won and
  lost stages. The default urgent queue includes only open-stage engagements.
- Stage settings group stages in open, won, and lost order. Managers can drag
  active stages within a group to reorder them or between groups to change their
  type. Archived stages remain visible through the status filter and are not
  draggable. Keyboard and no-JavaScript controls provide the same operations.
  A type change requires server-validated confirmation showing the stage, old
  and new types, and the count of all engagements in the stage.
  The server checks the full stage snapshot and engagement count in the same
  transaction as the change. It rejects a stale snapshot or count without
  changing the order or type. The change preserves engagement and history rows.
- The migration is forward-only. Recovery uses a pre-upgrade backup, consistent
  with the existing [upgrade procedure](operations.md#backup-and-restore).
- Stage setup guidance is role-specific. The file picker decision is recorded
  in [ADR 0015](decisions/0015-agreement-file-reference.md).

## Implementation decisions

These choices apply the approved requirements and existing conventions; they
are not additional business-policy approvals or claims of implemented behavior.

- **Names:** accept trimmed valid UTF-8, at most 200 Unicode code points, as
  for client display names (`internal/clients/service.go`). Migration builds
  the approved snapshot with the full client name and short ID; it must not
  silently truncate either. Allow generated names longer than 200 code points
  when necessary, and allow them to remain unchanged; enforce the limit on new
  or changed names. Reject invalid new names with a field-specific error. Do
  not enforce name uniqueness: the short ID distinguishes generated names, and
  users can intentionally choose the same name. Test long and non-ASCII names.
- **Existing actions:** retain each active row and its text, due date, and
  nullable owner; do not fabricate completion events. Membership removal can
  already clear an owner (`internal/engagement/next_action.go` and
  `internal/identity/service.go`). Complete an action and remove it from the
  active queue atomically. Replacement or clearing records the displaced action
  and its outcome, actor, and time in the same transaction. Reopening copies a
  completed action into the active slot without changing its history, and records
  the reopening actor and time as a separate event. If that slot is occupied,
  reject reopening without changing either action; the writer can clear or
  replace the active action first. A later completion creates a separate history
  entry. Reject stale or repeated transitions rather than duplicating history;
  test concurrent attempts and reopen cycles.
- **Dates and filters:** calculate today once per request in UTC. Overdue is a
  nonempty due date before today; due today equals today; upcoming is after
  today; undated has no due date. Default queue combines overdue and today.
  Completed is a separate history view, not mixed into active date buckets;
  apply owner filtering to the recorded owner, including a clear unassigned
  state after identity removal. The default urgent queue and its overdue and
  today buckets include only open-stage engagements, including occupants of
  archived open stages. Upcoming, undated, and completed filters include open,
  won, and lost engagements, including archived-stage occupants.
- **Export:** extend the existing bounded `dynamis-code.workspace/v1` JSON
  export with additive, profile-gated engagement collections, following the
  Clients export seam (`internal/portability/service.go`). Include stage and
  engagement identifiers and references, active actions, action history, and
  stage-change history with available actor identities; use public short IDs
  where defined, preserve nullable identities, and omit secrets. Count new
  records against existing export limits and fail rather than return a partial
  export. Specify exact fields and test both profile
  variants when implementing. Existing exports do not yet include these rows.
- **Interaction and scale:** use existing ordinary GET filters and bounded list
  pagination, with deterministic ordering and links retaining filters; do not
  claim a performance target without measurement. Use a server-validated
  confirmation for archiving with consequences visible without JavaScript,
  following `docs/web.md`. Stage drag-and-drop must retain accessible controls
  and a no-JavaScript fallback. Detailed visual design remains subject to the
  browser UX review.

## Implementation gates

Requiring both `name` and `stageId` on the existing REST v1 create API is
explicitly approved and breaks clients that omit either field. Do not make either
field optional or claim those clients remain compatible. No additional REST
policy choice is required.

The stage reorder REST v1 request is also a breaking change. Existing clients
that send only `ids` receive HTTP 400. Upgrade clients together with the server.
The request requires `ids`, the ordered stage `snapshot`, `movedId`, `newKind`,
`expectedCount`, and `confirmed`. A stale snapshot or count returns HTTP 409
`stale-stages`; an unconfirmed type change returns HTTP 409
`confirmation-required`. The snapshot contains each stage's `id`, `name`,
`kind`, and `archived` values in the order returned by the stage list.

Reopening a completed action conflicts with the one-active-action limit when
another action occupies the slot. The rejection rule above preserves both the
limit and history; it does not discard or replace the active action.

ADR 0015 permits the independently authorized picker with manual short-ID
fallback; it does not establish that a picker exists. The parent issue's older
statement that the ADR rejects it is superseded by the accepted ADR. No further
picker approval is needed for that architecture, but implementation and its
independent Files authorization remain gates.

A rendered-browser audit of the current journeys is required before declaring
#621 complete. Source inspection or automated checks do not replace desktop and
mobile, accessibility, keyboard, recovery, role, and no-JavaScript review.

Audit attempt at revision `cea28b0`: disposable SQLite bootstrap and server
readiness succeeded. Opening its local login page in Chrome was blocked because
browser origin approval was dismissed before a decision; no journey was
rendered or evaluated. Accessibility, mobile, and recovery findings remain
unknown. Repeat the audit with browser access before completing #621.

Audit attempt at revision `69376b5`: a fresh disposable SQLite instance passed
readiness and liveness checks, and its login response contained a form. The
previous claim that Playwright was unavailable was incorrect: Playwright is
available in the parent session. HTTP responses alone are not rendered-browser
evidence.

## Reported Playwright baseline

The user performed these observations with Playwright in the parent session,
using a disposable SQLite instance and an owner account. After signing in, the
user created an open Discovery stage, an Audit client, and an engagement.

- Client detail showed one unnamed engagement in Discovery. Stage, assignee,
  agreement, quote, and next-action forms were visible at the same time. The
  engagement creation flow showed no name or stage choice.
- The stage page grouped the workspace work-item label and stage creation. Its
  empty state said “No stages yet.”
- The empty actions-due page showed only a heading and an empty list, with no
  empty-state guidance.
- On client detail, entering next action `Call client` and due date `2020-01-01`
  with the owner unassigned, then selecting Save, returned HTTP 422 with the
  generic alert “Enter valid name and kind.” The due-date input was cleared.
- With next action `Call client`, due date `2020-01-01`, and owner
  `verify@example.com` selected, Save returned to client detail without an
  alert. Actions due listed only “Call client — 2020-01-01”, without client or
  owner context.
- Programmatically focusing Save next action and pressing Enter submitted without
  an alert. This checks Enter activation, not keyboard traversal or focus order.
- At a 390 × 844 viewport, document `clientWidth` and `scrollWidth` were both
  375. This sampled state had no horizontal document overflow.
- On mobile client detail, the first Tab focused “Skip to content.” Submitting
  an agreement with File ID `invalid-id` returned HTTP 422 and focused an alert
  under Client saying “Enter valid name and kind.” The returned form discarded
  the entered File ID, leaving it empty. This is one tested error, not a
  complete accessibility or recovery audit.
- In a separate JavaScript-disabled context using the existing owner session,
  creating an engagement increased the list count from 1 to 2.
- A separate viewer account created through an invitation saw the engagement
  region's stage, agreement, and quote summaries. That region contained zero
  buttons and zero comboboxes.
- A member registered through an invitation and created an engagement on client
  detail; the engagement count increased from 2 to 3.
- The owner submitted an invalid agreement File ID and saw a generic error.
  After checking signed agreement and resubmitting with an empty File ID, the
  owner returned to client detail without an alert. The exact displayed status
  was not verified.

Playwright captured a desktop screenshot in its MCP server output. The supplied
path `/var/folders/pt/yhlb769j4j1frcvj3mk_fy240000gp/T/pi-codemode-655cf9f4f999134a.png`
was inaccessible in this session (outside the permitted workspace); its
existence and visual contents could not be verified. No visual findings are
inferred. Desktop review and broader mobile review remain outstanding, as do
broader keyboard, error recovery, read-only role, no-JavaScript, and
comprehensive accessibility audits. The viewer and JavaScript-disabled
observations are sampled cases, not full coverage. The Enter activation did not
exercise keyboard traversal. These observations do not complete the #621
baseline audit; the later redesigned-journey review is a separate gate.

## Reported redesigned detail journey (#622)

The parent session exercised these samples with Playwright at revision
`90c529b`, using a disposable SQLite application and synthetic data. The owner
created a named engagement in an open stage and navigated to its detail page.
The detail sections were collapsed and the stage label was translated.

- On desktop, the owner renamed the engagement and saw “Engagement saved”.
- At 390 × 844, an invalid agreement file ID remained in the form after HTTP
  422, with the correct error shown. Document width showed no horizontal
  overflow in this sampled state.
- Keyboard sampling found the first Tab focused the skip link; Enter opened the
  name disclosure.
- In a separate JavaScript-disabled context, the owner renamed the engagement
  successfully.
- In a Spanish workspace at `90c529b`, stage, assignee, agreement, and
  next-action summaries were translated.
- A viewer saw the detail page without edit controls. This observation was
  made before later minor fixes, not at `90c529b`; its exact revision was not
  recorded.

Visual review used synthetic local data in the original workspace checkout.
Before the visual fix, desktop and mobile screenshots were captured locally at
`.playwright-mcp/engagement-622-desktop.png` and
`.playwright-mcp/engagement-622-mobile-closed.png`. Review found that edit
controls looked like static text, the navigation relationship was unclear,
blank notes added clutter, and sections lacked clear grouping. Commit `88ff69e`
adds outlined disclosures, an explicit back link, dividers, and hides blank
notes. A mobile after screenshot was captured locally at
`.playwright-mcp/engagement-622-mobile-after.png`; actual mobile before/after
review confirmed improved affordance. The next-action section falls near the
bottom of the viewport and requires scrolling. These local screenshot paths are
not committed.

The parent captured `.playwright-mcp/engagement-622-desktop-after.png` at
revision `1a5db57` and compared it with
`.playwright-mcp/engagement-622-desktop.png`. The explicit back link and
outlined controls improve recognition; dividers separate stages and sections.
Dense horizontal whitespace and the need to scroll remain subjective
observations. Sidebar contrast was not numerically verified. “Confirmed work”
and “Discovery” are synthetic user-entered values, not untranslated UI. Empty
notes are deliberately hidden when absent.

These are reported samples, not a complete journey matrix or accessibility
compliance assessment. The visual review does not establish accessibility smoke
checks or full UX completion. Independent review was not reported. The earlier
baseline and its blocked audit status remain separate; broader redesigned-journey
review remains outstanding.

## Reported integrated workflow samples (#619)

The parent session used Playwright with a disposable SQLite application at
baseline revision `cf59bd1`. The session created a stage,
client, and named engagement, then scheduled an overdue action.

- The dashboard showed “Overdue 1” and “Discovery 1”. The overdue link opened
  the correct contextual queue.
- Completing the action removed it from the active queue and showed it in the
  completed filter. Reopening it returned it to the overdue queue.
- The uploaded `agreement.txt` appeared by name in the independently authorized
  file picker. Selecting it saved the agreement.

After UI fixes at revision `f0fc252`, the parent sampled the mobile board at
390 × 844. The document had no overflow and showed no-target
guidance. Completing an action displayed “Action completed.” The parent
captured `.playwright-mcp/engagement-619-queue-mobile.png` in the original
checkout; the screenshot is local and not committed.

These are reported samples, not a comprehensive UX, accessibility, permission,
keyboard, error recovery, or no-JavaScript review. They do not close the
remaining audit gates.

## Additional rendered evidence (2026-11-24)

The following observations come from one reviewer using disposable SQLite
applications and synthetic data. They supplement, but do not replace, the
samples above. Screenshots are local, untracked files in the integration
worktree; they are not durable repository artifacts. File presence was checked
when this record was written, not their visual contents.

### Historical baseline: `cea28b0`

The reviewer exercised owner, member, and viewer journeys. A viewer's direct
agreement mutation returned HTTP 403; a member created a second engagement and
could not see stage-management controls. With JavaScript disabled, an owner
created an engagement and stage, moved the engagement, and recovered from one
invalid agreement submission. A mobile keyboard sample covered engagement
creation, but not the full editing journey.

An invalid agreement ID produced a generic error and cleared the entered value.
A 359-character unbroken note caused horizontal document overflow: at desktop
width 1280, client width was 1265 and scroll width 4114; at 390 × 844, client
width was 375 and scroll width 3827. The reviewer inspected the long-note
screenshot; the other two images were captured but not visually assessed.
Local screenshots:

- `.playwright-mcp/baseline-621-viewer-20261124.png`
- `.playwright-mcp/baseline-621-long-20261124.png`
- `.playwright-mcp/baseline-621-mobile-20261124.png`

Further historical samples used two similarly named clients. Queue links reached
the intended client on desktop and mobile, but the queue omitted client names;
two simultaneously active actions were not compared. The owner assigned and
cleared an assignee, updated a quote, changed and cleared an action, and moved an
engagement out of an archived occupied stage. Invalid quote and unassigned
action submissions showed a generic error and discarded drafts. A later
keyboard-only sample used sequential Tab, ArrowDown, End, and Enter to select
and submit a move with multiple active targets. Another sequential keyboard
sample submitted an invalid agreement ID: the generic error appeared, the
input cleared, and focus landed on BODY. Using Tab, Space, and Enter, the owner
then corrected and saved it. Focus visibility and screen-reader announcements
were not assessed.

Following each similarly named client from the list produced its distinct
heading and path. Archiving the client with an overdue action hid it from the
default client list, but its action remained in Actions due; unarchiving
restored the client with action and history intact. This records observed
existing behavior, not a decision that archived-client actions should remain
in the queue.

The reviewer recorded orientation, error recovery, and accessibility/resilience
observations as three lenses, not three independent reviews. Two more local
screenshots were captured but not visually inspected:

- `.playwright-mcp/baseline-second-client-desktop-20261124.png`
- `.playwright-mcp/baseline-second-client-mobile-20261124.png`

### Redesigned samples: `a170ed4`

An owner created a named engagement, scheduled an unassigned overdue action,
completed and reopened it, and selected an uploaded file in the agreement
picker. A viewer could read detail but a direct agreement mutation returned 403;
a member saw agreement editing but no Add stage button. A JavaScript-disabled
mobile sample retained an invalid manual file ID and completed a corrected
agreement submission. A sequential two-tab stale detail edit returned 409,
retained the draft, and succeeded on explicit retry.

At this revision action history labeled an unassigned owner “Unavailable”; an
invalid agreement ID contaminated the displayed summary; and stale queue
rescheduling discarded the submitted date. These observations are specific to
`a170ed4`, not claims about later revisions. Only one ready file was available:
picker pagination and cross-page draft preservation were not verified in the
browser. Local screenshots were captured but not visually assessed:

- `.playwright-mcp/redesign-621-detail-mobile-a170ed4.png`
- `.playwright-mcp/redesign-stale-conflict-a170ed4.png`
- `.playwright-mcp/redesign-picker-a170ed4.png`

### Focused recovery samples: `5b20bf3`

Two owner queue tabs exercised stale rescheduling: the recovery page showed the
current date and retained the submitted date; explicit retry saved the draft.
For a completed action, recovery retained the submitted date and offered no
reschedule button. An invalid date, injected after changing the native date
input to text, returned 422 and retained the input; a corrected retry saved.
The reviewer also observed “Owner: Unassigned” in action history and an invalid
agreement draft retained in the field without appearing in the saved summary.

Local screenshots were captured but not independently visually reviewed:

- `.playwright-mcp/queue-conflict-5b20bf3.png`
- `.playwright-mcp/queue-removed-5b20bf3.png`
- `.playwright-mcp/queue-invalid-5b20bf3.png`
- `.playwright-mcp/agreement-invalid-5b20bf3.png`

### Invalid stale replacement: exact commit `bc9e7ff`

One reviewer tested `bc9e7ffaf294c60f6483946580d90d099841060f`, exported with
`git archive` and run on a separate disposable server. A queue tab loaded the
original action at version 1; another tab replaced it. The reviewer changed the
native date input to text to submit an invalid date. HTTP 422 showed both the
invalid-date error and conflict warning, the replacement action and current
date, the invalid draft, and version 1. Correcting the draft and submitting
returned 409 with version 2; an independent detail reload confirmed the
replacement date had not changed. Explicit retry succeeded, and another detail
reload showed the corrected date.

Local screenshots were captured but not independently visually reviewed:

- `.playwright-mcp/bc9-invalid-stale.png`
- `.playwright-mcp/bc9-corrected-conflict.png`

None of these reports establishes full keyboard traversal, screen-reader
behavior, comprehensive role and lifecycle coverage, picker pagination,
independent review, or WCAG compliance. Historical baseline and redesigned
journey gates remain incomplete.

## Remaining current-journey audit

Status: **blocked pending rendered-browser review**. The following scenarios
cover the existing application, not the unimplemented redesign. Use a fresh
SQLite instance and synthetic data. Record the tested revision, viewport,
role, actions, outcomes, and durable screenshots or other evidence for each
scenario. Do not record credentials, invitation values, or session data.
Review each journey from the three perspectives in
[UX decisions](ux-decisions.md#review-perspectives); record whether reviewers
are independent. Record exact friction and its consequence, not only pass or
fail. Repeat any journey changed to address a finding.

1. As an owner on desktop and at 390 × 844, start at the workspace home and
   reach Clients, stages, and the due-actions list. Inspect empty states. Create
   two similarly named clients, multiple stages, and multiple engagements with
   distinguishable notes. Return through the available navigation and confirm
   which client and engagement each action affects. Inspect long names and
   notes, grouping, headings, and horizontal overflow.
2. On a client, create an engagement, move it between stages, change and clear
   its assignee and next action, and update its quote and agreement. Return to
   the client and stage history after each save. Check saved values, feedback,
   focus, and whether the next action is clear. Create overdue, today, future,
   and undated actions and compare the due-actions list with UTC dates and
   stage state. Inspect archived-stage occupants and attempts to move into an
   archived stage without assuming behavior that has not been observed.
3. Reproduce the invalid agreement File ID error on desktop and mobile. Record
   the message, focused element, field association, and submitted values after
   rejection. Correct the value and retry. Exercise another invalid field and a
   successful submission to distinguish a single error from a general pattern.
4. As a viewer, follow the same read paths and inspect summaries, history,
   empty states, and long content. Confirm that write controls are absent and
   record the outcome of a direct write attempt. As a member, inspect the
   available engagement actions and stage-management access; do not infer role
   coverage from the owner or viewer samples.
5. With JavaScript disabled, create and update an engagement, move its stage,
   submit an invalid value and recover, and inspect stage-management forms as
   an authorized owner. Check consequences and confirmation before any
   destructive action. The existing no-JavaScript creation sample does not
   establish these other paths.
6. On desktop and mobile, complete representative journeys using only the
   keyboard. Check visible focus, order, labels, error announcement and focus,
   and recovery. Run applicable accessibility checks and inspect their results;
   neither automated checks nor screenshots establish WCAG conformance.

Use these observations to inform redesign choices, not to claim the redesign
is implemented. After implementation, drive each changed journey again against
its acceptance criteria, including permission, failure, recovery, and
no-JavaScript states.

Non-browser baseline at revision `f535a5a`: `go test ./internal/engagement/...
./internal/web/...` passed. These package tests do not establish rendered
behavior or close the browser review gate.
