# Issue 655: partial browser review

**Status: blocked; review incomplete.** Tested revision `8135cd024bd59ee67f214c1bc8d0cb95c70051fd`. Isolated headless Chrome via Playwright; real server and disposable SQLite database. Screenshots contain synthetic workspace data only.

| Viewport / theme | Role | Action and observed outcome | Evidence |
|---|---|---|---|
| 1440 × 900 / light | Owner | Signed in, opened Create workspace disclosure; workspace name and language controls appeared. | [desktop-light.png](desktop-light.png) |
| 390 × 900 / dark | Owner | Repeated disclosure action; controls appeared at narrow width. | [mobile-dark.png](mobile-dark.png) |

The rendered disclosure carried `rounded-field border border-base-300`, and computed border style was `solid` in both cases. This verifies the target markup and a computed style, not stylesheet asset identity. The empty invitations and clients pages also rendered, but their changed controls require populated data.

`go test ./internal/web -run 'TestWorkspaceCreationDisclosureRespectsPermission|TestCurrencyNavigationAndViewerPermissions' -count=1` passed.

## Missing evidence

- No parent-revision screenshots or direct before/after comparison.
- No populated invitation revoke or list search/filter/column/saved-view journeys; no admin, member, or viewer drive.
- No asset-content comparison, keyboard/fallback pass, accessibility checks, error/recovery exercise, or full UX judgment. Screenshots and unit tests alone do not establish usability or accessibility.
