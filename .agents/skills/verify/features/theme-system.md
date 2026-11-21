# Theme system

`data-theme` on `<html>`: `light` and `dark` are explicit; the `system`
preference renders **no** `data-theme` attribute so daisyUI follows
`prefers-color-scheme`, matching the dark block in `assets/tokens.css`.
Mixed themes (light daisyUI cards with dark token ink) are the failure mode
this prevents.

## Sub-features

- Account theme preference (`/account` profile form, `theme` field).
- OS emulation: `prefers-color-scheme` light/dark per tab
  (`newTab(t, w, h, colorScheme)` in the harness).
- Contrast: every page must pass axe under both themes; the 44px form
  action button height is asserted by the accessibility smoke.

## How to get to it (user POV)

Account page -> theme select -> save; the app shell, auth pages, and public
pages all follow it immediately.

## Driving it with the go browser suite

```
go test -tags browser -count=1 ./internal/browsertest/... \
  -run 'TestItemsHonorsExplicitAndSystemTheme|TestLoginSystemThemeFollowsPreference|TestFilesSystemThemeFollowsOSPreference|TestAccountAndSecurityPagesRender'
```

Each case asserts the rendered canvas token (`--canvas` computed value:
`#f3f6fa` light, `#111826` dark) under each preference x OS scheme, via the
`expectTheme` helper (absence of `data-theme` for `system`). Theme-cookie
persistence is asserted by `go test ./internal/web/ -run TestAuditHistoryUsesProfileLocaleAndThemes`.

## Gotchas

- `system` never renders `data-theme` - asserting `data-theme="system"`
  is the old contract and will fail.
- The account profile save must POST `csrf`, `timezone`, `locale`,
  `display_name`, `theme` together (see `setAccountTheme` in
  `internal/browsertest/clients_list_test.go`).
- A stale workspace locale from a previous test in the same DB changes
  labels, not themes; don't chase contrast there first.
- Headless Chrome on this machine defaults `prefers-color-scheme: dark`;
  tabs that never emulate a scheme are dark-OS, not light.
