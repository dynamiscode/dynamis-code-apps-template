# Clients list screens

The heaviest list UI: live search, filter chips (step form), column
switches with drag order, saved views, bulk actions, sticky first column
and actions cell, and the mobile card view that replaces the table.

## Sub-features

- List: `.list-view-table` sticky name column, `td[data-label]` card view
  under 38rem; row menu `.list-action-items`.
- Search: `form.list-search` (role=search, live announce via
  `data-list-announce`).
- Filters: `client-query` step form, chips in `.list-filter-chips`.
- Columns: `.list-column-switches` with drag handles.
- Bulk: `.client-bulk.is-selecting` visibility, danger button.
- Field/value management on the client detail page (options, delete
  confirmations).

## How to get to it (user POV)

Workspace home -> Clients card; detail page per row; "Manage custom
fields" from the detail page.

## Driving it with the go browser suite

```
go test -tags browser -count=1 ./internal/browsertest/... \
  -run 'TestClients|TestClient'
```

covers list adaptation at 1280/390, draft persistence through refresh,
Spanish locale, detail + fields responsive layout with axe, field
lifecycle, option delete confirmations. Route-level proof without Chrome:

```
go test ./internal/web/ -run 'TestClientFieldLifecycle|TestClientsList'
```

## Gotchas

- `TestClientDetailAndFieldsResponsiveLayouts` creates a workspace-wide
  custom field ("Lifecycle note") that persists in the shared suite DB;
  later axe runs in the same suite see it. Solo runs use a fresh DB and do
  not - a contrast pass solo does not clear a suite failure there.
- Field labels are user data and do not follow the workspace locale; UI
  chrome does.
- The filter step form uses `data-dirty-track`; navigating away mid-step
  triggers the unsaved-changes warning.
