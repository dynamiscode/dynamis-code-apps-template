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
- For transaction-heavy work, lead with a scannable list and compact filters;
  keep setup and management distinct from routine recording and review.
- Identify records by meaningful context, not opaque IDs users must copy or
  remember. Put correction actions on the relevant record, preserve financial
  history, and show only actions allowed by the user's role.
- Make each record's descriptive title the detail link; legacy records without
  titles need a localized, navigable fallback.
- Present money as human-readable exact amounts with currency and correct
  precision. Never combine different currencies into one total or use
  floating-point rounding for financial values.
- Use the workspace's enabled currency list and default for new monetary
  records. Offer a labeled, searchable selection without accepting free-form
  codes, including an ordinary form path without JavaScript. Keep historical
  amounts and currencies visible even after a currency is disabled.
- Use predictable navigation: show where the user is, offer a clear way back,
  and keep destination labels consistent. Links navigate; buttons act.
  For detail and subpages, use the shared `molecules.BackLink` before the
  heading, preserving the originating list's filters and return context.
- Prefer recognition over memory. Show current values, status, ownership, and
  available next actions. Explain prerequisites where needed.
- Group related information; use progressive disclosure when it reduces work,
  not to hide essential status. Avoid repeating all edit forms for each list
  item. Choose the simplest familiar interaction that supports the task.
- Separate unrelated actions with visible space; adjacent controls without a
  gap should indicate an intentional group.
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
- For an empty list, center a descriptive, item-specific empty state with the
  permitted create action. Hide search, columns, and the duplicate toolbar
  create action. Show an archive filter only when archived records exist.
  Keep filtered-no-results states distinct from a genuinely empty list.
- In tables, make a linked value's full cell the link target. Highlight the
  cell and underline the text on hover or keyboard focus. Keep links distinct
  from buttons and do not make selection controls or row menus navigate.
  Reuse the same row-action menu across tables, including catalog. Highlight
  the entire action cell on hover or keyboard focus, and present each menu
  action as a labeled tile.
- Use shared list search, filter chips, sorting, columns, and saved views where
  supported. Keep filter state across navigation and apply it before pagination.
  Selecting a filter field opens its options without a page reload or request.
  Keep the filter menu's open state transient; a reload starts it closed.
  Show applied filters as wrapping pills: bold field and value, muted operator,
  and a borderless remove control. The pill label is not an interactive link.
  Column visibility controls need a submit button when JavaScript is disabled.
  Use the shared Columns menu directly in the list toolbar. Update its
  results and preferences in place, with a form fallback without JavaScript.
  During column dragging, show an opaque dragged item and an empty insertion
  gap. Keep keyboard and no-JavaScript reordering available.
  During stage dragging, highlight available groups and the current target,
  and label empty groups as drop targets. Offer a visible Move stage menu on
  touch devices, with keyboard and ordinary form alternatives.
  Do not add bulk selection to immutable history without a bulk action.
- Keep list tables in the shared list layout, without an extra surrounding card.
  Keep responsive row cards where the shared table uses them.
- Put list creation and its adjacent settings menu beside the page title, not
  in the search and filter toolbar. When space is insufficient, keep the
  title and menu on the first row and move creation below.
  Keep a visible gap between adjacent buttons, inputs, and cards. Use the
  same spacing conventions as other list and settings pages.
  Creation dialogs close when users click the backdrop, with the ordinary
  full-page form available when JavaScript is disabled.
- Detail dialogs retain the originating list context and offer a real close
  link; enhance it with modal keyboard and backdrop behavior when available.
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
- When creation and review share the same properties, align their labels and
  values alike. Keep quiet creation controls visibly focusable without JavaScript.
- Creation uses one unsaved draft across field editors, searches and pages; reveal
  a closed invalid field before focusing it. Detail edits confirm one field at a time.
- Replace a value in place while editing it; never append a second input below
  the saved value. Put optional creation fields and empty optional detail fields
  in Advanced; keep saved values visible and reopen Advanced for recovery.
- On detail pages, show concise saved values with clearly labeled edit actions.
  For independently saved single values, including text, numbers, and dates,
  use an in-place editor: the value reads as text with a right-aligned pencil
  inside the full-width clickable area. Hover highlights the whole area, and
  keyboard focus remains visible. On activation replace it in the same position
  without shifting surrounding content. Focus the input; select text where
  appropriate. Show accessible check and cancel icons in a floating toolbar
  only while editing. Enter saves when not operating a picker; Escape or cancel
  restores the saved value. Clicking outside closes the editor without saving
  or discarding the typed draft, and must not steal focus. Preserve clearing,
  validation, conflict recovery and an ordinary form without JavaScript.
  Reuse `molecules.EntryField` for creation and persisted entry controls. Empty
  labels act as placeholders and float above populated values. Choice lists open
  in a floating panel; lists over five choices provide live search. Draft fields
  remain local to their form until its submit action.
  Use theme-muted, readable placeholders and a decorative pencil for editable
  entry controls, including text, number, date, select and textarea fields.
  Selected defaults read as values until focused, without a permanent box.
  Align placeholder and floating label text with the entered or selected value.
  Preserve control geometry, clickable labels, visible keyboard focus and native
  select arrows and form operation without JavaScript.
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
- Use two columns when space permits. Reveal optional fields on demand, and
  reopen them when they contain a draft.
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
