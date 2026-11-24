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
  following `docs/web.md`. Use accessible stage controls first; omit optional
  drag-and-drop until separately justified and reviewed. Detailed visual design
  remains subject to the browser UX review.

## Implementation gates

Requiring both `name` and `stageId` on the existing REST v1 create API is
explicitly approved and breaks clients that omit either field. Do not make either
field optional or claim those clients remain compatible. No additional REST
policy choice is required.

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
not committed; no desktop after screenshot is claimed.

These are reported samples, not a complete journey matrix or accessibility
compliance assessment. The visual review does not establish accessibility smoke
checks or full UX completion. Independent review was not reported. The earlier
baseline and its blocked audit status remain separate; broader redesigned-journey
review remains outstanding.

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
