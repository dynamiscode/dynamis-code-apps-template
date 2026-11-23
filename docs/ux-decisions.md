# UX decisions for browser work

Use this guide with the [UX standard](ux.md) before building a new journey or
changing a meaningful interaction. Assess the whole task, not each control in
isolation. Product context and observed user needs take precedence over generic
examples; accessibility and security requirements remain binding.

## Decide before building

1. **Frame:** identify user, goal, entry point, context, constraints, success,
   and what happens next. List affected roles, states, and related journeys.
2. **Inspect:** trace existing navigation, components, data, and user paths.
   Existing code is a candidate, not proof of a good pattern. Identify where it
   helps or obstructs the task.
3. **Choose:** for consequential choices, compare at least two viable options
   against the criteria below. Consult the sources when the pattern is missing,
   questionable, unfamiliar, or complex. Record the chosen option, trade-off,
   and evidence in the change plan or PR. Routine controls need no comparison.
4. **Specify:** write observable acceptance criteria for each affected journey,
   including relevant failure and recovery paths. Resolve consequential product
   questions with the product owner rather than inventing policy.
5. **Verify:** follow [browser UX review](ux-review.md). Test assumptions with
   representative users where feasible; AI judgment is not user research.

## Criteria

| Area | Decide and check |
|---|---|
| Journey and navigation | Can users find the task, identify their location, return safely, and reach the next step? Choose navigation around users' tasks, not database structure. |
| Information and hierarchy | What must be visible to decide or act? Group related information, distinguish similar records, and put the primary task before secondary details. |
| Component and interaction | Does the control match the action and amount of content? A link navigates; a button performs an action. Consider direct editing, a separate page, or a dialog by context and interruption cost. |
| Disclosure and density | Is hidden content optional at this point? Keep essential status and actions visible. Avoid forcing users to open multiple sections to compare or complete a sequential task. |
| Forms and recovery | Are inputs clearly labeled, optionality and constraints clear, and values preserved on rejection? Explain errors near fields, offer a summary where appropriate, and provide cancel or recovery. |
| Feedback and state | Can users tell what is saved, pending, failed, or unavailable, and what to do next? Cover applicable empty, loading, success, error, permission, and long-content states. |
| Content and trust | Use familiar terms, specific action labels, and concise explanations. Make consequential actions, privacy implications, and irreversible outcomes clear before action. |
| Accessibility and context | Can people complete the same task with keyboard, assistive technology, zoom, touch, and narrow screens? Preserve focus, meaningful order, accessible names, and non-JavaScript paths. |
| Performance and scale | Can people find and act on work with realistic amounts of data? Check long text, repeated records, delay feedback, and list navigation without claiming unmeasured performance. |
| Visual coherence | Use typography, spacing, color, and grouping to make relationships visible. Reuse design tokens and components when suitable; improve a weak pattern rather than copying it blindly. |

## Pattern trade-offs

- **Page, inline edit, or dialog:** use a page when a task needs space, context,
  or a durable URL; inline edit for a small local change; a dialog for a bounded
  interruption. Do not hide a complex workflow in a dialog merely to avoid a
  page. Preserve context and focus on exit.
- **List and detail:** a list helps find and compare records; a detail view
  supplies context and actions for one record. Show distinguishing information
  in the list and provide a direct route to the selected record.
- **Tabs:** consider for clearly labeled related sections when the first is most
  useful and users do not need all sections at once. Avoid for sequential forms
  or content users need to read together. Check keyboard and fallback behavior.
- **Accordion or disclosure:** consider when people need only some related
  sections and evidence supports hiding them. Keep information everyone needs
  visible; test a clearly structured page without disclosure first.
- **Navigation:** use persistent service navigation when users repeatedly move
  between multiple tasks; a linear journey may need a simpler forward and back
  path. Preserve a clear current location.

## Review perspectives

Review the decision from three perspectives: (1) the user's task and journey,
(2) interaction and visual hierarchy, and (3) accessibility and implementation
constraints. Authorized independent reviewers can each own one perspective;
otherwise perform and record three distinct passes. Report any limitation; three
passes by one agent are not independent reviews.

## Sources

- [GOV.UK: navigate a service](https://design-system.service.gov.uk/patterns/navigate-a-service/)
- [GOV.UK: tabs](https://design-system.service.gov.uk/components/tabs/) and [accordions](https://design-system.service.gov.uk/components/accordion/)
- [GOV.UK: recover from validation errors](https://design-system.service.gov.uk/patterns/validation/)
- [USWDS: forms](https://designsystem.digital.gov/components/form/)
- [W3C: WCAG 2.2 quick reference](https://www.w3.org/WAI/WCAG22/quickref/)
- [Nielsen Norman Group: usability heuristics](https://www.nngroup.com/articles/ten-usability-heuristics/)
