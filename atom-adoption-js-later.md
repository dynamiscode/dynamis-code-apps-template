# Atom adoption — JS-coupled items (later)

Tracked separately from atom-adoption-findings.md: these replacements touch JavaScript
behavior, so they need browser verification (`make browser-smoke`, `make accessibility-smoke`)
and coordinated JS changes, not a markup-only swap.

| Item | Where | What blocks a markup-only swap | Atom/extension needed |
|---|---|---|---|
| Row/action menus | `internal/web/components/list_table.templ` ListRowMenu, `list_actions.templ` ListActionsMenu, `list_views.templ` switcher | `list-table.js:109-112` targets `details.list-row-menu` + `:scope > .list-menu-panel` (outside-click close, panel positioning); `.list-menu-panel` CSS positions the panel | atoms.Dropdown needs `SummaryAttrs` (aria-label, title, glyph) and a panel slot; then update list-table.js selectors |
| ListSearch | `list_table.templ` ~204-223 | App search is a GET form with preserved hidden fields, `forms.js` autosubmit, `data-error-text` + live regions; atom is htmx-only (hx-get, keyup, spinner) | atoms.ActiveSearchInput needs a plain-form mode |
| Column switch | `list_table.templ` ~262-279 | App switch is a submit `button role=switch` (hx-post, disabled for locked columns); atom is a checkbox + inline script | atoms needs a button-switch variant |
| Toolbar/`list-add-menu`/`list-create-menu`/`list-settings` details | `list_table.templ`, `list_views.templ` | Same `list-table.js` menu selector list; panels are forms, not menus | atoms.Dropdown with form children (Item slot already exists) + list-table.js selector update |
| WebMCP export anchor + notification plumbing | already converted (Anchor/Badge) | none — listed only to note `data-webmcp-export` flows through Attrs | done |

Rule of thumb: any swap that changes a selector `list-table.js`, `forms.js`, or `app.js`
queries needs the JS updated in the same change, plus a browser run.
