# Component action review (#657)

Inventory baseline: `aa927e2` and locally available `origin/main` (`0824a31`).
The current integration branch was `aa927e2` at review. Production journey
ownership remains #652 (request forms), #653 (shared navigation and lists),
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

**Gaps:** screenshots and automated checks are not a complete interactive UX
review. The fixture does not exercise the nested megamenu popovers, every
variant, all tab stops, or real application journeys. Other theme/viewport
combinations and no-JavaScript interaction were not reviewed. The Playwright
shared profile was occupied; this run used a separate chromedp browser instead.
Real-application UX review remains outstanding; this issue is not complete.
