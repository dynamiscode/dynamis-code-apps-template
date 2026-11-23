# Visual design review

Review the rendered screen at realistic sizes and content lengths. Use the
application's existing design tokens and components when they serve the task;
consistent appearance cannot excuse an unusable interaction.

- **Hierarchy:** Identify what draws attention first, next, and last. Does that
  order match the task, current state, and risks? Reduce competing primary
  actions without hiding necessary information.
- **Grouping and layout:** Related labels, controls, errors, and actions should
  read as a unit; separate unrelated tasks. Check alignment, spacing, reading
  order, and whether long content breaks the relationship.
- **Typography and copy:** Use a consistent scale, legible body text, clear
  headings, and predictable alignment. Check actual translation, wrapping,
  zoom, and line length rather than enforcing a universal character count.
- **Color and state:** Use semantic tokens; verify light and dark themes,
  contrast, focus, selected, disabled, and error states. Do not rely on color
  alone. Do not assert contrast passes without measuring it.
- **Responsive behavior:** Compare desktop and narrow screens with realistic
  content. Keep actions reachable, ensure focus is visible, and test actual
  touch or keyboard interactions when those are part of the claim.
- **Motion:** Make transitions explain a change rather than delay a task;
  respect reduced-motion preferences and preserve feedback without motion.

Record the observed element and user impact, not claims such as 'looks bad' or
an unsupported numeric quality score. Recheck the affected viewport after a
fix. User testing and accessibility checks remain separate evidence.

## Sources

[USWDS typography](https://designsystem.digital.gov/components/typography/) ·
[USWDS design principles](https://designsystem.digital.gov/design-principles/) ·
[W3C WCAG 2.2 quick reference](https://www.w3.org/WAI/WCAG22/quickref/) ·
[design-audit repository](https://github.com/wonjyou/design-audit) (review
categories, not evidence of user outcomes)
