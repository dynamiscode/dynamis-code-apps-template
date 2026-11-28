# Browser UX review

Required for new browser journeys and meaningful interaction or visual changes.
Backend-only changes and copy-only changes without usability impact are exempt;
state why. This is a completion gate, not an automated CI guarantee. An
independent reviewer is useful when available and authorized, but no particular
AI tool or model is required.
For pull requests touching `internal/` or browser-related commands, CI requires
a structured review record or a reasoned non-visual exemption. A reviewed
record must name the current PR head SHA or an unambiguous prefix of at least
seven hex digits. Update it after new commits and repeat the drive as applicable.
The check validates field shape, required state classifications, commit identity,
and HTTPS link syntax. It does not fetch links or establish that artifacts are
durable, accessible, or genuine; nor does it validate reasons, completeness of
the journey inventory or path filter, or usability. Reviewers must judge those.

The lightweight `ux-review` check runs on pull request creation, updates,
reopening, and body edits without restarting CI. Configure branch protection to
require `ux-review` before relying on it to enforce body edits. If the original
CI accessibility job failed because the record was incomplete, rerun that job
after correcting the record; the lightweight check does not replace it.

1. **Inventory.** List affected journeys, entry and return paths, roles, and
   relevant states. Identify task, success, failure and realistic data, including
   multiple similar records. Note a state as not applicable only with a reason.
2. **Design.** Follow [ux-workflow](../.agents/skills/ux-workflow/SKILL.md)
   and the [UX decision guide](ux-decisions.md). Challenge existing patterns;
   compare options and consult sources when needed. Define observable
   acceptance criteria for each changed journey.
3. **Drive.** Run the real application with disposable data using the
   repository's [verification procedure](../.agents/skills/verify/SKILL.md).
   Complete each changed journey, including entry, action, outcome, and return,
   not just visit a page or run isolated tests. Inspect desktop and narrow
   screens and sample applicable empty, populated, error, success, permission
   and long-content states. Exercise permitted and restricted roles, keyboard,
   visible focus, assistive technology where applicable, and ordinary form
   fallback without JavaScript.
4. **Judge.** Apply the three perspectives in the [UX decision guide](ux-decisions.md).
   For each journey assess navigation and orientation, task priority,
   recognition, information grouping, choice and control, feedback, error
   recovery, consistency, and responsive behavior. Name exact friction and the
   consequence for the user; fix findings in scope and repeat the affected drive.
   Review rendered screens, not source or automated scores alone.
   For financial journeys, check exact amounts and currency precision, separate
   currency totals, and correction history with realistic records. Assess whether
   routine work starts with a scannable list, whether management distracts from
   it, and whether users must find or copy opaque identifiers to act.
5. **Record.** In the change report or pull request give tested revision,
   viewport, role, journey and state, actions and outcomes, links to screenshots
   or other durable evidence, checks run, and unresolved findings. Use synthetic data;
   do not record credentials, personal data, or session values.

For a reviewed pull request, list each affected journey with its own six state
lines, using exactly these forms:

```text
- Journey: Record a transaction
- State: empty | exercised | Opened history before adding a transaction | https://example.org/review/empty
- State: populated | exercised | Found the new transaction in history | https://example.org/review/populated
- State: error | not applicable | This journey does not expose an error state
- State: success | exercised | Saved the transaction and saw confirmation | https://example.org/review/success
- State: permission | exercised | A restricted role could not save | https://example.org/review/permission
- State: long-content | exercised | Read a long transaction description on a narrow screen | https://example.org/review/long-content
```

Replace the examples with actual observations and review-accessible durable
artifact links. The same artifact may cover multiple states. An exercised state
needs its action and outcome plus a link; a not applicable state needs a
specific reason and no artifact field. Use `blocked` for a state that applies
but cannot be exercised; a reviewed record cannot contain a blocked state.
Do not relabel a blocked state as not applicable to pass the gate. A reason is
necessary but reviewers must decide whether it is valid. Repeat the journey and
all six state lines for every affected journey. The exemption form remains
`Status: exempt` with a reason.

## Capture and retain evidence

Capture the real application's rendered state during the drive, including the
action and outcome in the review record. Use an available approved browser;
`@Browser` is not required. When the Playwright browser tool is available,
`browser_take_screenshot` accepts a `filename` for a PNG of the current page.
Inspect the image and record its viewport and state. If the active browser cannot
save an image, use another approved browser with screenshot capture, or record
the evidence gap as blocked.

The verification procedure's `/tmp/verify-evidence/` directory is staging, not
durable storage. Before cleanup or handoff, attach the synthetic-data screenshots
and relevant logs to the pull request or another review-accessible durable
artifact store. Link each artifact from the review record and check that a
reviewer can open it. Do not publish HTML, logs, or screenshots until checked for
credentials, personal data, session values, and other sensitive content.

If the app, browser, or required state cannot be exercised, mark review
**blocked**, explain the gap, and do not claim the UI work complete. Automated
checks and screenshots alone do not establish usability or WCAG conformance.
