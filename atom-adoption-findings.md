# Atom adoption — remaining work

Status after the adoption pass (all Go tests, lint, node unit tests green). Implemented
rows have been removed; this file tracks only what is left.

## Remaining to implement (no new JS needed)

| Item | Where | What is needed |
|---|---|---|
| Navigation menus (WorkspaceContext, AccountMenu) | `internal/web/layouts/navigation.templ` | Two-line role markup (`strong` + `span`), POST logout form in the menu, nav-label headings; extend atoms.Dropdown with a summary slot + form Item (Item slot already exists), or accept simpler standard look |
| Sidebar / PeopleTabs | `internal/web/layouts/navigation.templ` | atoms.Sidebar is a bare aside shell; NavLink (active + aria-current) has no atom — decide whether to add `NavLink` atom or keep layout-local |
| DataTable header cells → atoms.HeaderText | `list_table.templ` ListHeader | Headers are templ children today; moving to `[]atoms.SelectOption`-style data (or HeaderText components) is a mechanical pass once decided |
| Webhooks table headers | `internal/web/webhooks.templ` | Same as above; table itself is already on atoms.TableWrap |

## JS-coupled (see atom-adoption-js-later.md)

Row/action menus, ListSearch, column switch, toolbar details panels.

## Done in this pass

- Field helpers: TextField, TextareaField, SelectField (removed; all 10 SelectField call
  sites now build `[]atoms.SelectOption`), ErrorAlert, SuccessAlert, SubmitButton,
  ConfirmButton, LinkButton on atoms.
- Breadcrumbs: account header, workspace header, audit/export crumbs on atoms.Breadcrumbs
  (new `Href` field; daisyUI breadcrumbs look replaces the `/` separators).
- ListPager on atoms.LinkPagination; `.list-pager*` CSS removed (daisyUI join look).
- DataTable/AuditDataTable/ListTable and the webhooks delivery table on atoms.TableWrap
  (new atom; caption, region attrs, pin-rows preserved).
- Filter operator radios on atoms.Radio; `.client-operator` CSS removed.
- LinkedCard on atoms.ListCard (new `li.card` list variant in atoms/card.templ).
- CSS cleanup: `.list-pager`, `.list-pager-gap/status`, `.client-operator`,
  `.loading::before` (double spinner with atoms.Loading), plus 11 orphan class rules.
