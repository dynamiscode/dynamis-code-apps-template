# User experience standard

Use this standard for browser journeys and apply [UX review](ux-review.md)
before declaring meaningful interaction or visual changes complete. Product
owners define their users and goals; generated applications may change visual
identity without weakening accessibility or the task outcomes below.

## Design for the task

- Identify the user, their goal, starting point, and successful outcome. Use
  realistic content and the existing product vocabulary, not backend fields as
  the information architecture.
- Make the primary task and current context apparent. Put the information
  needed for a decision next to the action; distinguish multiple similar
  records by useful names and context.
- Use predictable navigation: show where the user is, offer a clear way back,
  and keep destination labels consistent. Links navigate; buttons act.
- Prefer recognition over memory. Show current values, status, ownership, and
  available next actions. Explain prerequisites where needed.
- Group related information; use progressive disclosure when it reduces work,
  not to hide essential status. Avoid repeating all edit forms for each list
  item. Choose the simplest familiar interaction that supports the task.
- Prevent mistakes without obstructing routine work. Explain consequential
  actions, provide cancel or recovery where appropriate, and preserve unsaved
  input on validation failure. Give specific errors near fields and a summary
  where needed; confirm success and make progress visible.
- Design empty, loading, success, error, permission, and long-content states
  when applicable. Indicate what users can do next, including when they cannot
  perform an action.
- Use existing components, tokens, and ordinary forms; keep behavior consistent
  across desktop, narrow screens, keyboard, assistive technology, and without
  JavaScript. Follow [accessibility](accessibility.md) and [web](web.md).

## Sources

- [Nielsen Norman Group: 10 usability heuristics](https://www.nngroup.com/articles/ten-usability-heuristics/)
- [W3C: WCAG 2.2 quick reference](https://www.w3.org/WAI/WCAG22/quickref/)
- [W3C: form notifications](https://www.w3.org/WAI/tutorials/forms/notifications/)
- [GOV.UK: recover from validation errors](https://design-system.service.gov.uk/patterns/validation/)
- [USWDS: forms](https://designsystem.digital.gov/components/form/)

These sources inform judgment; conformance to a checklist alone does not prove
usability. Validate with representative users when possible.
