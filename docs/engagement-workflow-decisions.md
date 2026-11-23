# Engagement workflow decisions (#621)

These are approved requirements for the redesign in #619, not claims about
current behavior. Existing behavior remains documented in [capabilities](capabilities.md),
[API](api.md), [web](web.md), and [data lifecycle](data-lifecycle.md).

## Approved

- Existing engagements receive an editable name generated from a snapshot of
  the client name followed by ` — ` and the engagement short ID. The name is
  required, including in the existing REST API. A later client rename does not
  change the snapshot.
- Date boundaries use UTC.
- An engagement has one active action. Completion produces immutable history
  visible to readers; existing writers manage active actions.
- Archived stages remain visible, including their occupants; moves out are
  allowed but moves into archived stages are not. Dashboard counts include work
  in open stages, including archived-stage occupants.
- Deletions cascade. Workspace export includes engagements and completed
  action history.
- Stage setup guidance is role-specific. The file picker decision is recorded
  in [ADR 0015](decisions/0015-agreement-file-reference.md).

## Remaining decisions and implementation gates

The approved snapshot format does not settle name length limits, truncation,
collision handling, or migration rollback. Completion history's exact fields,
completed-action filters, handling of deletion of actors, and migration of
existing active actions require implementation design without inventing new
policy. Export format and its stage and active-action coverage require review.
Dashboard pagination and performance limits, precise archive confirmation,
optional drag-and-drop, and visual interaction design remain unresolved. No
approval for additional business-rule changes is implied.

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

Playwright saved a screenshot in its MCP server output. Its durable location and
visual contents were not verified. Desktop review and broader mobile review
remain outstanding, as do broader keyboard, error recovery, read-only role,
no-JavaScript, and comprehensive accessibility audits. The viewer and
JavaScript-disabled observations are sampled cases, not full coverage. These
observations do not complete the required rendered-browser audit.
