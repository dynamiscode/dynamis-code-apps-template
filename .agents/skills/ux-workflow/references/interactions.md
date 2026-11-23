# Interactions and states

Check the full action lifecycle, not just the happy path. Keep normal forms and accessible controls usable without JavaScript.

## Controls and input
- Use links for navigation and buttons for actions. Pick controls that match the decision and number of choices; do not require precise memory or manual identifiers when a safe, available selection pattern fits.
- Group related fields, use persistent labels, show optionality and constraints, and choose defaults based on known context. Avoid asking for information already known.
- Choose inline editing for small local changes, a page for work needing space or a durable location, and a dialog for a bounded interruption. Test keyboard, focus on entry and return, and the fallback for the selected pattern.

## Feedback and recovery
- Show current state and results of actions. For slow work, show progress where useful and prevent accidental repeat submissions without trapping users.
- Preserve attempted values after validation failure. Identify errors in text, associate them with fields, and provide a focusable summary when appropriate. Explain how to recover; do not blame the user.
- Provide cancel or undo when appropriate. Explain consequences before destructive or irreversible actions and preserve authorization checks in shared policy.

## States and roles
- Assess empty, first use, populated, many records, long content, loading, partial success, timeout, failed, unavailable, stale, and permission-limited states when relevant. For each state identify what users see, can do, and how they recover or continue.
- Distinguish read-only information from editable controls and explain blocked prerequisites. Check touch, keyboard, focus visibility, screen-reader names and order, reduced motion, zoom, localization, and narrow-screen reflow.
- Check honest defaults, consent, and consequences. Do not use misleading urgency, preselected consent, or visual pressure to steer an unintended choice.
- Avoid promising real-time updates, offline support, undo, or file access unless implemented and authorized. Treat personal or sensitive data in screenshots and evidence as a security concern.

## Sources
- [W3C: form notifications](https://www.w3.org/WAI/tutorials/forms/notifications/)
- [GOV.UK: recover from validation errors](https://design-system.service.gov.uk/patterns/validation/)
- [USWDS: forms](https://designsystem.digital.gov/components/form/)
- [W3C: WCAG 2.2 quick reference](https://www.w3.org/WAI/WCAG22/quickref/)
