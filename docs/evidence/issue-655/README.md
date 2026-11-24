# Issue 655: browser review evidence

**Status: partial; full UX review remains incomplete.** Baseline `aa927e2`; target `8135cd024bd59ee67f214c1bc8d0cb95c70051fd`. Two independently built real servers, separate disposable SQLite databases and isolated headless Chrome contexts via Playwright. Data is synthetic; screenshots omit invitation links, passwords and session values. Desktop is 1440 × 900 / light; mobile is 390 × 900 / dark (system color preference).

## Settings and catalog

For each revision the owner invited and registered a viewer, created a manual-price catalog entry named Consultation, and opened General settings and the populated catalog. Both roles then opened currencies from General settings and opened the catalog entry. Currency and detail screenshots are desktop/light. Catalog creation succeeded on both revisions; viewer catalog omitted Add entry and Edit entry, and viewer currency screen omitted owner currency-edit controls. These observations cover rendered controls, not direct mutation authorization.

| Role / viewport | General settings: before → after | Catalog: before → after |
|---|---|---|
| Owner / desktop | [baseline](base-owner-desktop-settings.png) · [target](target-owner-desktop-settings.png) | [baseline](base-owner-desktop-catalog.png) · [target](target-owner-desktop-catalog.png) |
| Owner / mobile | [baseline](base-owner-mobile-settings.png) · [target](target-owner-mobile-settings.png) | [baseline](base-owner-mobile-catalog.png) · [target](target-owner-mobile-catalog.png) |
| Viewer / desktop | [baseline](base-viewer-desktop-settings.png) · [target](target-viewer-desktop-settings.png) | [baseline](base-viewer-desktop-catalog.png) · [target](target-viewer-desktop-catalog.png) |
| Viewer / mobile | [baseline](base-viewer-mobile-settings.png) · [target](target-viewer-mobile-settings.png) | [baseline](base-viewer-mobile-catalog.png) · [target](target-viewer-mobile-catalog.png) |

| Role | Currencies: before → after | Catalog detail: before → after |
|---|---|---|
| Owner | [baseline](base-owner-currency.png) · [target](target-owner-currency.png) | [baseline](base-owner-detail.png) · [target](target-owner-detail.png) |
| Viewer | [baseline](base-viewer-currency.png) · [target](target-viewer-currency.png) | [baseline](base-viewer-detail.png) · [target](target-viewer-detail.png) |

Keyboard: on both revisions, Tab reached Manage currencies with a visible 3px solid focus outline; Enter opened Currencies. [Baseline focus](base-keyboard.png) · [target focus](target-keyboard.png). Enter also opened the catalog detail after focusing its link programmatically. This is a focused keyboard check, not a full accessibility audit.

General settings renders notification preference Save for the viewer on both revisions. This review did not submit it or determine whether authorization denies that operation; do not infer write access from its visibility.

## Asset identity

HTTP responses for all four stylesheets matched SHA-256 of the corresponding files used to build each instance. Target `components.css` also matched the file at `8135cd0`, and differed from baseline:

| Asset | Target SHA-256 |
|---|---|
| `tokens.css` | `d844e66bab3f67a7cbf1193f99bdc1aa13a4c5dc8d7a78403f4080f81a5e310e` |
| `components.css` | `4b77731c23761e16b8105b3e2fe928b3ca8bea401fdd9e5fe3bb586ac6b9c6fe` |
| `app.css` | `6909e1fe30aaf930e580492fff4d6e730f538797502e115a4fbe202f0b7f8a8b` |
| `clients.css` | `2fc2a3fc19c4653ffe8af819bf176abfda0e7ce90167d3f60fa2317c4d6e765d` |

Baseline `components.css`: `29ac5558317ab695add60729df836b78fc1afde456b9cc41611c40c6d8de7adc`.

## Earlier target-only check

Owner opened Create workspace and saw name and language controls: [desktop/light](desktop-light.png), [mobile/dark](mobile-dark.png). The disclosure had the expected border classes and computed solid border. `go test ./internal/web -run 'TestWorkspaceCreationDisclosureRespectsPermission|TestCurrencyNavigationAndViewerPermissions' -count=1` passed.

## Invitation and list follow-up

Tested committed revision `4f8bf6b`, built from a separate source archive, against a fresh disposable SQLite database and isolated headless Playwright browser. Owner role, 1440 × 900 light. Images contain synthetic records only; invitation links were not captured.

- Created a viewer invitation and reloaded the list to remove the one-time link: [pending](invitation-pending.png). Revoke without the required confirmation was blocked by native form validation. Space checked the confirmation, and Enter on Revoke submitted it: [confirmed](invitation-confirm.png), [revoked](invitation-revoked.png).
- Opened a separately revoked link in a signed-out browser: [unavailable invitation](invitation-unavailable.png). The outlined Home link led to `/`, which redirected the signed-out user to `/login`; activated with Enter after focusing it programmatically.
- Created two similarly named clients. Tab reached Columns with a solid 3px focus outline; Enter opened its menu: [columns](list-columns-keyboard.png). Enter in the search field for `Studio` reported one result: [search](list-search.png). Enter opened Views after focusing its summary programmatically: [views](list-views.png).

## Remaining gaps

No admin or member journey, list filter or saved-view mutation, ordinary form fallback, complete keyboard traversal, or automated accessibility audit was performed. No screen-reader or contrast conformance claim is made. Viewport images sample the journeys; they do not establish full UX or WCAG conformance. The invitation and list follow-up does not include mobile or baseline comparison.
