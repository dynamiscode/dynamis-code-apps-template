# Component action review (#657)

Inventory baseline: `aa927e2` and locally available `origin/main` (`0824a31`).
Re-inventoried integration tip `66ee529`: its changes since `aa927e2` cover
account, authentication, invitations, notifications, sessions, public intake
and public pages (#656); client fields, clients, engagement list/board/detail
(#654); and shared list controls (#653). No preview or screenshot fixtures
were added in that integration range. Production journey ownership remains #652 (request forms), #653 (shared navigation and lists),
#654 (clients and engagement, including newly added engagement screens),
#655 (workspace tools and settings), and #656 (account and public journeys).
Final cross-slice enforcement belongs to #658.

## Reviewed surfaces

- Preview `/components`: fixture index links now have boundaries; alert fixtures
  remain unchanged. Preview workspace states belong to #655.
- Screenshot fixtures: alert fixture has no navigation. Screenshot renderer
  remains unchanged.
- Catalog atoms: feature and footer links, menu and megamenu links, megamenu
  trigger, and step links have visible boundaries. Breadcrumbs, dropdowns,
  status return links and pagination already have boundaries. Anchor and Link
  remain configurable catalog primitives, including their supported link variant.
  Whole-card links and application list controls belong to #653.
- Production pages and feature-gated states remain assigned to the slices above;
  this review does not certify their completeness.

## Evidence and gap

Focused render tests fail before the changes for unbounded feature, footer,
menu, step, megamenu, and preview-index links and pass after.

Isolated headless Chrome, on integration `66ee529` merged with `6e2502c`,
rendered synthetic catalog components without accounts or personal data:

- [Desktop light, 1280 × 900](reviews/issue-657/desktop-light.png).
- [Mobile dark, 390 × 844](reviews/issue-657/mobile-dark.png).

`go test -tags browser -count=1 -run TestCatalogNavigationBrowser -v ./cmd/preview`
rendered the feature, footer, menu, step, megamenu, and supported Link variant.
Tab focused Learn more; Enter navigated to `#reports`. The test checked for
horizontal overflow. Screenshots capture the keyboard-focused state. Fixture
links and labels are synthetic. The view demonstrates bounded navigation beside
an intentionally unbounded supported Link variant.

## Real application and no-JavaScript review

On `66ee529` merged with `f9dbd0c`, the real-app browser suite booted a
fresh synthetic SQLite workspace and isolated Chrome. `TestSignInKeyboardFocus`
checked visible focus through the mobile sign-in recovery link. In
`TestIdentityRolesWithoutJavaScript`, sign-in rejected a wrong password,
accepted the synthetic owner, and navigated account → sessions at 390 × 844.
See [keyboard sign-in](reviews/issue-657/mobile-login-keyboard-recovery.png),
[no-JS login](reviews/issue-657/mobile-nojs-login.png), and
[no-JS account](reviews/issue-657/mobile-nojs-account.png).

`TestPublicIntakeLongConsentReflow` exercised the real public form, validation,
keyboard focus and long consent at 1440 × 900 light and 390 × 844 dark; it
submitted successfully with JavaScript disabled. See [desktop form](reviews/issue-657/desktop-light-form.png),
[mobile keyboard](reviews/issue-657/mobile-dark-keyboard-consent.png), and
[no-JS success](reviews/issue-657/mobile-nojs-success.png). Screenshots contain
synthetic fixture data only.

## Remaining blocker

At 390 × 844 with JavaScript disabled, Enter opens the catalog fixture's outer
native megamenu popover, but its nested trigger has computed `display: none`
and cannot receive focus (`Element is not focusable (-32000)`). See
[opened menu](reviews/issue-657/mobile-megamenu-nojs.png). The nested keyboard
journey is skipped, not passed; root cause is not yet confirmed. The fixture is
not used by a production page. Other theme/viewport combinations and complete
variant coverage remain unreviewed. Issue #657 remains incomplete.
