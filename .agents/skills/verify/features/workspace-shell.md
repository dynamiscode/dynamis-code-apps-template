# Workspace shell

The dashboard frame: sticky navbar with workspace switcher and account
menu, the daisyUI drawer that becomes the sidebar from `lg` up, the skip
link, and the mobile drawer toggle.

## Sub-features

- Navbar: brand link, breadcrumb context, workspace dropdown
  (`details.dropdown-end`), account menu with sign-out form button.
- Drawer: `div.drawer.lg:drawer-open`, checkbox `#app-drawer`, overlay is a
  real `<button class="drawer-overlay">`, toggle is
  `button[aria-label="Open navigation"]` wired to `pageBehaviors.toggleDrawer`.
- Skip link: `a.skip-link`, first Tab stop on every page.
- Settings sidebar and member pages share the navigation markup.

## How to get to it (user POV)

Sign in and land on the workspace home; the navbar is the top band, the
sidebar is visible on desktop, hidden behind the hamburger on mobile.

## Driving it with the go browser suite

```
go test -tags browser -count=1 ./internal/browsertest/... \
  -run 'TestSettingsKeyboardReachableAndAxeClean|TestMembersLayouts'
```

Proves: axe clean on the shell, keyboard reaches nav, drawer toggles,
focus behavior after swaps, 390px reflow. Server-rendered proof without
Chrome:

```
go test ./internal/web/ -run 'TestWebAccountNotificationsAndPasswordReset|TestWorkspaceMode'
```

asserts the drawer/navbar markup markers. For the aria-label string see
`internal/i18n/locales/en.json` (`ui.open_navigation`).

## Gotchas

- The drawer overlay must stay a `button` with `tabindex="-1"` and
  `aria-hidden` - a `label` there failed axe (`aria-prohibited-attr`,
  `region`) once already.
- Sign-in lands on the workspace of the seeded owner only when
  `MULTI_WORKSPACE_ENABLED` matches the harness; without it the home
  redirect differs.
- The suite asserts exact class strings on back-links
  (`btn btn-ghost btn-sm w-full justify-start font-normal`); markup edits
  must update `internal/web/handler_test.go` in the same change.
