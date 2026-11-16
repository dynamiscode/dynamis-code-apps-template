# UI measurement method and asset inventory

Issue #446 establishes the method for the screen baseline in #439. It does not
measure screens or set budgets. Repeat measurements on the same commit and
report changes to the environment or fixture with the results.

## Isolated fixture and browser conditions

The existing `scripts/playwright-smoke.sh` creates a fresh temporary SQLite
database, bootstraps one synthetic owner and one workspace, builds the server,
binds it to loopback, and removes the database and server on exit. Initially it
contains zero Items, Clients, Files, invitations, and notifications. The smoke
journey creates one Item; rerun the script for a fresh fixture. It does not seed
the populated, permission, upload, sharing, MFA, or failure states needed by
#439. Record the exact synthetic setup and counts for each additional state;
do not reuse a database modified by another scenario. Never copy production data.

Run from the repository root with Go, Node, npm dependencies and Chrome installed:

```sh
npm ci
make browser-smoke
```

The harness uses `127.0.0.1:58095` by default; set `PLAYWRIGHT_PORT` if occupied.
It uses Playwright's Chrome channel, one worker, `en-US`, reduced motion, and
1280×900 by default. Its screenshot cases also use 390×844 and light/dark
system color schemes. For performance runs, record viewport, device scale,
locale, account locale, theme and reduced-motion setting explicitly. Use a new
browser context for each cold run, with an empty HTTP cache and service workers
blocked; use a separately labeled repeat navigation in the same context for a
warm run. Sign-in may warm assets, so clear the browser cache after sign-in
before the measured cold navigation without clearing the session. Record any
deviation, network throttling, run count and aggregation; do not compare runs
with different conditions as equivalent.

Capture request count and encoded transfer bytes from browser network events,
and navigation timing from the Navigation Timing API. Separate document
navigation, HTMX fragments, SSE connections/events and uploads/downloads;
long-lived SSE must not be treated as a completed page load. Record definitions,
start and end markers, errors, redirects, cache hits, loading feedback and
unavailable metrics. Do not infer transfer bytes from asset gzip sizes below.
The existing smoke command verifies the fixture and journeys; it does not collect
these performance metrics. Screen-specific instrumentation and results remain
work for #439.

Use this result format for each run or aggregate:

```text
commit: <SHA>; date UTC: <ISO timestamp>; OS: <version and architecture>
browser: <name/version>; Playwright: <version>; command: <exact command>
fixture: <creation command, synthetic counts and state>; route/interaction: <name>
viewport: <width x height, device scale>; locale: <browser/account>; theme: <value>
cache: <cold/warm and method>; network: <conditions>; runs: <count/aggregation>
requests: <count and boundaries>; transferred bytes: <value/method or unavailable>
timings: <metric, unit, start/end and value or unavailable>
loading/failure: <observed state or unavailable>; exclusions: <reason>
```

Do not record credentials, cookies, authorization headers, CSRF values, secret
URLs, response bodies, traces or screenshots containing secrets. Keep raw traces
local and inspect them before sharing. Report unavailable data explicitly rather
than substituting zero.

## Embedded asset inventory

Snapshot: 2026-11-16, commit `81c496187ec060f359a38e844859c23c5e265931`, Darwin 27.2.0
arm64, Go 1.27.4, Node 26.10.0, Chrome 157.0.8103.78, Playwright 1.64.1.
The browser and fixture conditions above describe the intended screen runs;
no browser performance measurements were collected for this inventory.

Production CSS and JavaScript under `internal/web/assets` are embedded by
`internal/web/handler.go`. Test `.mjs` files and the HTMX license are not
production CSS/JS. The app uses system fonts; no font files are embedded.
First-party assets have no separate version: their version is the commit above.
`components.css` is built with Tailwind CSS 4.3.5 and daisyUI 5.8.1;
`htmx.min.js` is HTMX 4.0.1. Sizes are bytes, with deterministic gzip level 9
(`gzip -n -9`); this is a comparison size, not a claim of HTTP compression.

| Asset | Raw | gzip |
|---|---:|---:|
| app.css | 29422 | 5646 |
| clients.css | 2873 | 806 |
| components.css | 16347 | 3183 |
| import.css | 579 | 266 |
| app.js | 14398 | 4061 |
| forms.js | 22633 | 6960 |
| htmx.min.js | 37448 | 13390 |
| mfa.js | 6470 | 2108 |
| notifications.js | 850 | 344 |

Reproduce the inventory from the root:

```sh
git rev-parse HEAD
uname -smr
go version
node --version
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --version
for f in internal/web/assets/*.css internal/web/assets/*.js; do
  printf '%s raw=' "$f"; wc -c < "$f"
  printf 'gzip='; gzip -n -9 -c "$f" | wc -c
done
```

Use the local Chrome executable on other platforms. Brotli sizes, actual HTTP
compression, per-screen requests and timing, additional fixture states, and
budgets are not measured here.
