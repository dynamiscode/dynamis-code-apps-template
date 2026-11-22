# Feature map

One file per user-facing feature: what it is, how a user reaches it, how to
drive it with the repo's harness, and what end state proves it works. When a
journey needs a precondition the seed data lacks, report the failure and put
the reason in that feature's Gotchas - never weaken the journey.

| Feature | File | Suite drive (`-run`) |
|---|---|---|
| Workspace shell (drawer navbar, skip link, navigation) | [workspace-shell.md](workspace-shell.md) | `TestSettingsKeyboardReachableAndAxeClean`, `TestMembersLayouts`, `TestWorkspaceHome...` |
| Theme system (light/dark/system, contrast) | [theme-system.md](theme-system.md) | `TestLoginSystemThemeFollowsPreference`, `TestFilesSystemThemeFollowsOSPreference`, `TestAccountAndSecurityPagesRender` |
| Clients list screens (filters, columns, bulk, cards) | [clients-list-screens.md](clients-list-screens.md) | `TestClientsList*`, `TestClientDetail*`, `TestClientField*` |
| File upload (progress, retry, lifecycle) | [file-upload.md](file-upload.md) | `TestFilesUploadProgressRecoversAndRetries`, `TestFilesPassAxe`, `TestFilesViewerListsAndDownloadsWithoutUploadControls` |

Sign-in for every drive: `playwright@example.com` /
`playwright-password-123` (owner). Sign-in is itself verified by
`TestAuthenticationPagesUseBrandAndOrdinaryFormPaths` and the login suite.
