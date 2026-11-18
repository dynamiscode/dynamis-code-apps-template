# UI measurement method and asset inventory

Issue #446 established the method for the screen baseline in #439. This page
records the shared fixture and asset inventory; the [administration evidence](ui-measurement-admin.md)
contains measured settings and data-management screens. The inventory below
maps all requested screens and interactions, with unmeasured coverage marked.
No performance budgets are set. Repeat measurements on the same commit and
report changes to the environment or fixture with the results.

## Isolated fixture and browser conditions

The existing `scripts/playwright-smoke.sh` creates a fresh temporary SQLite
database, bootstraps one synthetic owner and one workspace, builds the server,
binds it to loopback, and removes the database, local file storage and server on
exit. Bootstrap and server run with empty inherited environments and explicit
fixture configuration; external telemetry, S3, OIDC and email remain disabled.
The Go build and browser tools retain the caller's environment. Initially it
contains zero Items, Clients, Files, invitations, and notifications. The smoke
journey creates one Item; rerun the script for a fresh fixture. It does not seed
the populated, permission, upload, sharing, MFA, or failure states needed by
#439. Record the exact synthetic setup and counts for each additional state;
do not reuse a database modified by another scenario. Never copy production data.

Run from the repository root with Go, Node, npm dependencies and Chrome installed:

```sh
npm ci
make browser-smoke
UI_MEASURE=1 ./scripts/playwright-smoke.sh
node --test scripts/playwright-isolation.test.mjs
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
The measurement command signs in, clears Chrome's HTTP cache with CDP, then
measures workspace-home navigation in a new context and repeats it warm. It
prints JSON without paths, headers or response bodies. Each result counts CDP
request starts from navigation until the home marker is visible and timing is
read after the load event, including redirects; completed encoded
bytes sum `Network.loadingFinished.encodedDataLength` in that interval.
Unfinished requests, including long-lived streams, are counted separately and
their bytes are unavailable. Redirect bytes may be incomplete; HTTP responses
with status 400 or higher and transport failures are counted separately. HTTP
error details contain only the status, an allowlisted resource type and a
`favicon` or `other` classification; URLs and query strings are never emitted.
Redirects and CDP cache-served events are also reported. Timings are Navigation Timing milliseconds from
navigation start to DOMContentLoaded and load. This is one unthrottled run,
not an aggregate or a complete per-screen baseline. Adapt the script for each
interaction and report its exact command and boundaries; HTMX, SSE, uploads,
downloads and loading feedback remain unmeasured by this navigation procedure.

On 2026-11-16, a validation run against base commit `830c8f2096ec08e6fd7aa65656b96c3380bd6683`
with the diagnostic change applied reported one cold HTTP 404 for the favicon
(CDP type `Other`), and no warm HTTP errors or transport failures. This is one
observation, not a screen baseline or a performance budget.

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

## Screen and interaction inventory (#439)

The route grouping below follows [web rendering migration inventory](web.md#rendering-migration-inventory)
and current route tickets. “Measured” links to recorded evidence. Existing
behavior or accessibility tests are not performance measurements. State names
describe applicable UI states; unmeasured means no timing, request or transfer
capture exists. Do not infer zero cost or absence from those cells.

| Screen or interaction | Existing behavior evidence / owner | Performance and state coverage |
|---|---|---|
| Workspace home | `scripts/playwright/ui.spec.mjs`; #439 | Cold/warm navigation measured below; empty fixture only. Populated, pending, error, permission and no-JS states unmeasured. |
| Items list, create, edit | `scripts/playwright/ui.spec.mjs`; #413–#416 | Navigation and create/edit interaction timing, fragment/SSE request counts and bytes unmeasured. Empty, populated, validation, error, permission and no-JS performance states unmeasured. |
| Clients list and detail | `scripts/playwright/clients-list.spec.mjs`; #415 | SSE refresh and validation behavior tested; performance unmeasured. Empty, populated, pending, failure, permission, detail, and no-JS measures absent. |
| Files list and upload | `scripts/playwright/files.spec.mjs`; #417 | Behavior tested; navigation, upload/download transfer and feedback timing unmeasured. Empty, pending, validation, failure, permission and no-JS measures absent. |
| Members and invitations | `scripts/playwright/members.spec.mjs`, `invitations.spec.mjs`; #419, #420 | Behavior tested; list/creation/acceptance navigation and mutation timings unmeasured. Empty, pending, validation, failure, permission and no-JS measures absent. |
| General/email settings, tokens/SCIM, webhooks/deliveries, audit, import/export | [Administration evidence](ui-measurement-admin.md); #422, #423, #424, #425, #426 | Cold/warm navigation and selected audit, import-preview and export interactions measured. Detailed states and limits are in linked evidence. |
| Account, security, sessions, notifications | `scripts/playwright/settings.spec.mjs`, `notifications.spec.mjs`; #427, #429 | Behavior tests exist; navigation and interactions unmeasured. Empty, pending, validation, error, permission and no-JS measures absent. |
| Setup, login, MFA, recovery | `scripts/playwright/ui.spec.mjs`, `password-reset.spec.mjs`; #430 | Behavior tests exist; cold/warm timing and request counts unmeasured. Invalid credentials, MFA, validation, errors, pending and no-JS measures absent. |
| Public share and consent | `scripts/playwright/ui.spec.mjs`; #431 | Behavior tests exist; navigation and consent interaction unmeasured. Empty, pending, failure, permission and no-JS measures absent. |
| Loading, empty, pending, validation, error, permission, no-JS across routes | Route-specific tests above | Selected admin state observations exist; other states and cross-screen loading feedback durations are unmeasured. |

No per-screen performance budgets or fixes are proposed. Asset raw/gzip sizes
are a shared inventory, not attribution to each screen. Current navigation
measurement counts all requests in the interval; it does not identify initiator,
script ownership, or whether repeat requests are unnecessary. HTMX, SSE,
upload/download wire bytes and loading feedback require separately bounded
interaction measurements. See route owners above for follow-up; no behavior
change is claimed.

### Earlier workspace home navigation observation

On 2026-11-17T23:27:16.556Z, commit
`1c2ff7f59d106988976daf1370e52463f60d0896`, Darwin 27.2.0 arm64,
Go 1.27.4, Node 26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1:

```sh
PLAYWRIGHT_PORT=58951 UI_MEASURE=1 ./scripts/playwright-smoke.sh
```

One unthrottled loopback run used a fresh SQLite fixture with one synthetic
owner, one workspace and zero Items. Browser conditions were 1280×900, device
scale 1, `en-US`, light theme, reduced motion, service workers blocked. Cold
cleared the HTTP cache after sign-in; warm immediately repeated the route.
Counts cover request starts and completed encoded bytes until load and the home
marker. Navigation Timing supplies DOMContentLoaded and load. One cold favicon
404 occurred; warm had no HTTP errors. No redirects, transport failures,
cache-served events or unfinished requests occurred. Results are one sample,
not a distribution.

| Cache | Requests | Completed encoded bytes | DOMContentLoaded ms | Load ms |
|---|---:|---:|---:|---:|
| Cold | 5 | 98293 | 13.4 | 15.4 |
| Warm | 4 | 97781 | 13.5 | 13.6 |

This earlier run measures workspace-home navigation only. It does not measure
other routes, screen interactions, populated states, loading duration,
compression over HTTP or per-resource attribution. The cold favicon 404 is
observed and not fixed here; route ownership needs triage before assigning a
specific ticket.

### Current workspace home navigation observation

On 2026-11-18T00:40:57.686Z, commit
`a0088c636dcaf8d3073dffe1e936b12e96b0ac65`, Darwin 27.2.0 arm64,
Go 1.27.4, Node 26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1:

```sh
PLAYWRIGHT_PORT=58131 UI_MEASURE=1 ./scripts/playwright-smoke.sh
```

One unthrottled loopback run used a fresh SQLite fixture with one synthetic
owner, one workspace and zero Items. Browser conditions were 1280×900, device
scale 1, `en-US`, light theme, reduced motion, service workers blocked. Cold
cleared the HTTP cache after sign-in; warm immediately repeated the route.
Counts cover request starts and completed encoded bytes until load and the home
marker. Navigation Timing supplies DOMContentLoaded and load. One cold favicon
404 occurred; warm had no HTTP errors. No redirects, transport failures, cache
events or unfinished requests occurred. Results are one sample, not a
distribution.

| Cache | Requests | Completed encoded bytes | DOMContentLoaded ms | Load ms |
|---|---:|---:|---:|---:|
| Cold | 5 | 98293 | 12.2 | 13.7 |
| Warm | 4 | 97781 | 10.4 | 11.9 |

This run measures workspace-home navigation only. It does not measure other
routes, screen interactions, populated states, loading duration, compression
over HTTP or per-resource attribution. The cold favicon 404 remains unassigned
to a fix ticket.
