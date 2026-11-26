# User experience standard

Use this standard for browser journeys and apply [UX review](ux-review.md)
before declaring meaningful interaction or visual changes complete. Product
owners define their users and goals; generated applications may change visual
identity without weakening accessibility or the task outcomes below.

Use the [UX decision guide](ux-decisions.md) to compare patterns and turn
these principles into journey-specific acceptance criteria.

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
- On detail pages, keep current work in the main column. Use a narrower
  sidebar for context that helps the task. On narrow screens, stack the
  main content first and offer a link to the supporting section.
- Combine related events into one activity section when they tell one story
  and share access rules. Keep current actions separate. Show event type,
  actor, and time when available. Preserve corrections and older entries.
  Add filters only when volume makes the feed hard to scan.
- Build activity feeds from existing sources of truth. Authorize each event
  and scope reads to the workspace. Order and paginate the combined results
  with a stable tie-breaker. Record new events only when needed; define their
  retention, export, deletion, and sensitive-data rules.
- Default lists to active records. Offer archived records through the list's
  filters instead of separate Active and Archived buttons. Preserve other
  search, filter, and sort settings when the archive filter changes.
- In tables, make a linked value's full cell the link target. Highlight the
  cell and underline the text on hover or keyboard focus. Keep links distinct
  from buttons and do not make selection controls or row menus navigate.
  Reuse the same row-action menu across tables, including catalog. Highlight
  the entire action cell on hover or keyboard focus, and present each menu
  action as a labeled tile.
- On detail pages, put secondary record actions in a reusable three-dot menu
  at the top right. Use labeled tile actions and show only actions permitted
  for the current user and record. Keep destructive confirmations.
  Align the menu within the viewport on narrow screens and above nearby content.
  Keep the menu keyboard accessible and usable without JavaScript.
- For short detail properties, align labels in a left column and values in a
  second column on wider screens; stack smaller labels above values on narrow
  screens. Use semantic label/value markup, wrap long values, and give longer
  notes their own full-width section. Show absent values as localized “Not set”
  in muted, readable text; keep the whole empty value area editable for writers.
  Do not distinguish empty values by color alone.
- On detail pages, show concise saved values with clearly labeled edit actions.
  For independently saved single values, including text, numbers, and dates,
  use an in-place editor: the value reads as text with a right-aligned pencil
  inside the full-width clickable area. Hover highlights the whole area, and
  keyboard focus remains visible. On activation replace it in the same position
  without shifting surrounding content. Focus the input; select text where
  appropriate. Show accessible check and cancel icons in a floating toolbar
  only while editing. Enter saves when not operating a native picker; Escape,
  cancel, or a click outside restores the saved value without a confirmation
  alert. Native picker interaction must not count as an outside click. An
  outside click must not steal focus from the control the user selected.
  Preserve clearing, validation and conflict recovery, and a usable ordinary
  form without JavaScript.
  Keep independently saved finite-choice selectors visible with the current
  value selected. When the user chooses a different value, open a confirmation
  dialog naming the field and proposed value. Confirming saves the selection;
  cancel, Escape, or clicking outside the dialog restores the previous value.
  Show errors and preserve recovery when a save fails.
  Use a dialog for related fields that must be edited together, without
  duplicating the saved summary beside the active form. Keep ordinary forms for
  creation, filters, search, and fields submitted together.
- For Markdown notes, show rendered content until the user activates editing.
  Use a shared rich-text editor that displays formatting while editing and
  preserves Markdown features it cannot edit. Keep a plain Markdown form for
  users without JavaScript. Provide accessible Apply and Cancel controls.
  Enter inserts a newline; Escape or clicking outside cancels without saving.
  Preserve rejected drafts and offer conflict recovery without overwriting
  newer content. Sanitize rendered content and validate links.
  Keep the toolbar visible in Markdown source mode and offer a return to visual
  editing. Disable formatting actions in source mode. If the draft cannot be
  represented without loss, keep its source and explain why visual editing is
  unavailable.
  Enclose the formatting toolbar, editor, and Apply and Cancel controls in one
  visible editing boundary. Do not place controls over the editor border.
- Match the interaction to the data. Keep read-only history distinct from
  editable properties. Do not split an atomic update into independent
  submissions that can overwrite concurrent changes.
- Dialogs and inline editors retain drafts on errors, restore focus when closed,
  and prevent accidental loss of unsaved input. Preserve an ordinary form path
  without JavaScript, including validation and confirmation.
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
