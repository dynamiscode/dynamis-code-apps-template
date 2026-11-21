---
name: templ-components
description: How this repo structures its server-rendered templ component library - the atoms/molecules/organisms/templates packages under internal/web/components, props structs with Class and Attrs, marker classes for tests and JS, and the generate/drift workflow. Use when adding or changing any templ component, page, or shell, or when deciding where a new piece of markup belongs.
---

# templ components

## Package layout

Four Go packages under `internal/web/components/`, atomic design, manual
constructor injection (no DI):

- `atoms` - one element or one small control: `Button` (`btn` + props.Class),
  `Input`, `Select`, `Textarea`, `Label`, `Field` (fieldset + legend +
  error note), `Alert`, `Badge`, `Dropdown`, `TableWrap`, `Progress`,
  `ListCard`, `ListRow`, `ToggleInput`.
- `molecules` - composed controls: `SubmitButton`, `ConfirmButton`,
  `LinkButton`, `ErrorAlert`, `TextField`, `SkipLink`, `ListSearch`,
  `PageHeading`, `EmptyState`.
- `organisms` - feature regions: `DashboardHeader`, `AccountHeader`,
  `Navigation`, `SettingsSidebar`, `ListScreen`, `ListTable`,
  `ListFilters`, `ListViews`, `AccountMenu`, `WorkspaceContext`.
- `templates` - document shells: `UiHead`, `Workspace` (drawer shell),
  and the page skeletons.
- Feature pages stay at the `internal/web` root next to their handlers
  (`items.templ`, `clients.templ`, `auth.templ`, ...), importing the four
  packages. `viewmodel.PageData` is the single data carrier templates read.

Downward imports only: pages -> templates -> organisms -> molecules ->
atoms. An atom never imports a molecule.

## Props conventions

- Every component takes a props struct with `Class string` appended to
  the base classes (`<button class={ "btn", props.Class }>`) and
  `Attrs templ.Attributes` spread last.
- Callers pass translated text; components never read locales directly.
- Keep semantic **marker classes** (`settings-panel`, `session-card`,
  `list-search`, `auth-card`) alongside the styling classes. Unit tests
  assert exact class strings, browser tests and `alpine-components.js`
  select by them, and the accessibility smoke finds elements by them.
  Dropping a marker breaks them silently - grep before removing one.

## Workflow

1. Edit `.templ`, then `go tool templ generate -path internal/web`.
2. Zero drift: `go tool templ generate -check -path internal/web` - run
   before commit; committed `_templ.go` must match.
3. Markup assertions live in `internal/web/*_test.go`
   (`strings.Contains` on exact class/id strings). Update the assertion
   and the markup in the same change - a class edit without its test
   edit is an incomplete change.
4. Checks, in the cheap-first order:
   `make js-check` (assets syntax), `go test ./internal/web/...`,
   `make test-unit`, then the browser suite
   `go test -tags browser -count=1 ./internal/browsertest/... -run '<names>'`.

## Adding a new feature surface

1. New controls go in the lowest package that needs nothing above it.
2. Compose pages from organisms; do not hand-roll shell markup in a page -
   call `templates.Workspace` / `organisms.DashboardHeader`.
3. Every form field goes through `atoms.Field`/`molecules.TextField` so the
   label, `aria-describedby` error association, and the fieldset note
   reservation come for free (see the `form-markup` and `form-field-input`
   skills for the rules).
4. Destructive buttons use `molecules.ConfirmButton` with
   `data-confirm`; never a bare button.
