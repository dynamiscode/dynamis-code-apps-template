# Choose interaction patterns

Start with the user's task, not a preferred component. For consequential
choices, compare alternatives against task, data, frequency, roles, device,
accessibility, fallback, and cost of errors. Check existing components and
public design-system guidance; existing code and external advice are candidates,
not proof. Record assumptions and validate uncertain choices with users when
feasible. Do not turn a design-system convention into a universal law.

| Need | Candidates and trade-offs | Check |
|---|---|---|
| Find known work | Search, filters, or navigation | Keep the query and active filters visible; distinguish no data from no matches and offer a way back. Preserve context when returning from a record. |
| Compare records | Table, list, or cards | Pick a structure that exposes the attributes being compared; on narrow screens preserve access to labels, values, and actions. Test long and duplicate names. |
| Navigate a complex workspace | Persistent navigation or a simpler task path | Organize by users' goals; use a clear location and return path. Avoid deep or hidden navigation when it impedes discovery. |
| Change a value | Inline edit, dedicated page, or dialog | Compare context, complexity, interruption, return path, keyboard, and fallback. Keep edits associated with the correct record and preserve unsaved work. |
| Choose options | Radio buttons, checkboxes, select, or search | Distinguish single from multiple selection; compare visibility, number of choices, known labels, keyboard operation, and mobile usability. Avoid exact numeric thresholds without evidence. |
| Read related content | Headings, tabs, or disclosure | Show essential information by default. Avoid hiding content users must compare or read sequentially. |
| Wait or recover | Inline status, progress, retry, or a safe exit | Reflect actual operation and progress; distinguish partial success, validation, permission, timeout, and network failure when relevant. Never imply unsupported retry or offline behavior. |
| Confirm a risky action | Review, confirmation, undo, or combinations | Explain consequences and affected scope; choose recovery proportional to risk. Do not promise reversal if unavailable. |

## Sources and confidence

- [GOV.UK navigation](https://design-system.service.gov.uk/patterns/navigate-a-service/), [tabs](https://design-system.service.gov.uk/components/tabs/), and [accordions](https://design-system.service.gov.uk/components/accordion/) describe context-specific patterns.
- [W3C WAI form notifications](https://www.w3.org/WAI/tutorials/forms/notifications/) and [WCAG 2.2 quick reference](https://www.w3.org/WAI/WCAG22/quickref/) inform accessible feedback and interaction.
- [Pixel-Ridge UX patterns](https://github.com/Pixel-Ridge/ux-research-skills/tree/main/skills/ux-patterns) separates research strength from design-system convention. Treat evidence grades as the authors' assessments; follow cited primary sources before making a strong claim.
- [Awesome UX Skills](https://github.com/tommyjepsen/awesome-ux-skills) inventories heuristic, journey, and cognitive-load review approaches; its suggestions are not established facts. Do not assume fewer clicks or a specific layout always improves outcomes.
- [UX Skills](https://github.com/opentoolsorg/ux-skills) discusses state inventories and recovery. Adapt the method to actual supported behavior instead of adding unsupported features.
