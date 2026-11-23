# Lists, search, filters, and bulk work

Choose a structure for the user's task, data volume, and comparison needs; do
not assume a table, card, or board is always best. Keep authorization and
workspace scope in shared policy.

## Find and compare

- **List or table:** expose meaningful identity and attributes needed to
  distinguish records. Use a table for row/column comparison, with meaningful
  headers and a caption or accessible name; do not use tables for layout.
- **Search and filters:** retain the query and applied filters; provide ways to
  remove them and distinguish a genuinely empty workspace from no matches. Do
  not reveal inaccessible records through counts or suggestions.
- **Sort and paging:** communicate current sort and page; apply query, filters,
  and ordering consistently across the result set. When filters change, avoid
  leaving the user on an invalid page. Returning from a detail view should retain
  useful list context where supported.
- **Narrow screens:** retain labels, values, identity, and actions. Compare
  container-scoped horizontal scrolling with a stacked presentation; neither is
  universally right. Test long text, zoom, keyboard, and actual data density.

## Selection and batch actions

- Make selection state and its scope clear, including whether it includes only
  the page, filtered results, or all results. Offer a clear way to cancel.
- Explain how many records are affected, permissions, irreversible consequences,
  and any partial success or failure. Preserve actionable feedback and safe
  recovery; do not claim an undo or retry that does not exist.
- Test selection across page/filter changes, duplicate submissions, access
  changes, and mixed valid/invalid items when applicable. Never widen the actual
  server-authorized selection based on visual state alone.

## Evidence

[GOV.UK tables](https://design-system.service.gov.uk/components/table/) ·
[GOV.UK pagination](https://design-system.service.gov.uk/components/pagination/) ·
[Carbon data tables](https://www.carbondesignsystem.com/building-blocks/core/components/data-table/guidelines)
