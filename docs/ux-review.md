# Browser UX review

Required for new browser journeys and meaningful interaction or visual changes.
Backend-only changes and copy-only changes without usability impact are exempt;
state why. This is a completion gate, not an automated CI guarantee. An
independent reviewer is useful when available and authorized, but no particular
AI tool or model is required.

1. **Inventory.** List affected journeys, entry and return paths, roles, and
   relevant states. Identify task, success, failure and realistic data, including
   multiple similar records. Note a state as not applicable only with a reason.
2. **Design.** Follow [ux-workflow](../.agents/skills/ux-workflow/SKILL.md)
   and the [UX decision guide](ux-decisions.md). Challenge existing patterns;
   compare options and consult sources when needed. Define observable
   acceptance criteria for each changed journey.
3. **Drive.** Run the real application with disposable data using the
   repository's [verification procedure](../.agents/skills/verify/SKILL.md).
   Complete each changed journey, not just visit a page. Inspect desktop and
   narrow screens and sample applicable empty, populated, error, success,
   permission and long-content states. Check keyboard, visible focus, and
   ordinary form fallback. Run applicable accessibility and behavior tests.
4. **Judge.** Apply the three perspectives in the [UX decision guide](ux-decisions.md).
   For each journey assess navigation and orientation, task priority,
   recognition, information grouping, choice and control, feedback, error
   recovery, consistency, and responsive behavior. Name exact friction and the
   consequence for the user; fix findings in scope and repeat the affected drive.
   Review rendered screens, not source or automated scores alone.
5. **Record.** In the change report or pull request give tested revision,
   viewport, role, journey and state, actions and outcomes, links to screenshots
   or other durable evidence, checks run, and unresolved findings. Use synthetic data;
   do not record credentials, personal data, or session values.

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
