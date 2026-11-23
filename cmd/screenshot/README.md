# Offline fixture screenshots

Use this workflow for fast visual inspection of a page or component with synthetic data. It does not require a database, login, or application server.

## Agent workflow

1. Find the real templ page or component and its data inputs. Choose the states and viewport sizes relevant to the change.
2. Reuse a registered fixture or add one in `cmd/screenshot/main.go`. For an unexported page, add a synthetic-data adapter in `internal/web` like `ScreenshotLogin`. Use synthetic data only. Register each state you need.
3. Run the command below with the fixture name, theme, dimensions, and distinct output paths. Use a temporary directory for generated files.
4. Open the PNG with the image-reading tool and inspect the rendered result; do not infer appearance from HTML or a successful command. Repeat for relevant narrow and desktop viewports and states. Share the PNG path when useful.
5. Report the fixture, viewport, theme, observed result, and any rendering limitations. For meaningful UI changes, still complete the real-app journey required by [UX review](../../docs/ux-review.md); mock screenshots do not prove interaction, authorization, or usability.

## Command

Render a registered synthetic templ component or real templ page to self-contained HTML and a viewport PNG:

```sh
go run ./cmd/screenshot -fixture alert -theme dark -width 1280 -height 800 -html /tmp/alert.html -png /tmp/alert.png
```

Use `-fixture login` for the real sign-in page with synthetic data. Chrome or Chromium must be installed and discoverable by chromedp. Themes: `light`, `dark`, `system` (browser preference). Dimensions are CSS pixels. Defaults: `alert`, `light`, 1280 × 800, `fixture.html`, `fixture.png`.

Register components in `fixtures` and full documents in `pages` in `main.go`. Full page constructors may need a small exported synthetic-data adapter in `internal/web`; never use real credentials or data. The `login` fixture uses the actual sign-in templ page. Embedded `/assets/*.css` and `/assets/*.js` references become data URLs, preserving script order and attributes; rebuild after asset changes. Missing or unsupported static resource references fail with their path. CSS imports and non-data CSS URLs, `srcset`, inline style elements, and non-CSS/JS embedded assets are unsupported. Fragment fixtures remain static and reject scripts. Full pages execute their embedded scripts, but CSP blocks connections, form submissions, and external resources; Chromium also blocks network URLs. Runtime-created missing resources are blocked but not reported as unresolved assets. No server or database is started; server-dependent interactions and realtime data cannot work.

Run `go test ./cmd/screenshot` with Chrome installed to test HTML, asset rejection, and PNG dimensions.
