# UI measurement method and asset inventory

Issue #446 established the method for the screen baseline in #439. This page
records the shared fixture and asset inventory; [administration evidence](ui-measurement-admin.md)
contains settings and data-management screens, while [identity evidence](ui-measurement-identity.md)
contains account, authentication and public screens. The inventory below maps
all requested screens and interactions, with unmeasured coverage marked.
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
#439. The core measurement creates one synthetic Client, one Item and one
File during its run. Record the exact synthetic setup and counts for each
additional state;
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
`UI_MEASURE=1 ./scripts/playwright-smoke.sh` signs in, clears Chrome's HTTP
cache with CDP, then measures workspace-home navigation and repeats it warm.
`UI_MEASURE=identity ./scripts/playwright-smoke.sh` runs the identity navigation
and interaction measurements in [identity evidence](ui-measurement-identity.md).
`UI_MEASURE=core ./scripts/playwright-smoke.sh` measures cold/warm empty Items,
Clients and Files navigation, creates a synthetic Client and measures its
cold/warm detail page, then measures synthetic Item creation and edit,
empty-title browser validation, and uploads a synthetic 256 KiB file. It uses the same isolated
fixture and emits aggregate JSON. Upload feedback timing includes automation
overhead. `requestBodyBytes` counts request-body bytes from CDP `postData` when
present, otherwise from a known `Content-Length`; bodies without either are
counted separately. The upload runner queries CDP for the synthetic upload
request body. It emits and stores no headers or payloads. Counts exclude HTTP
headers and framing, so they are body sizes rather than full wire bytes. Server
storage is discarded with the fixture.
`UI_MEASURE=members ./scripts/playwright-smoke.sh` measures cold/warm Members
and Invitations navigation, then one synthetic invitation creation. It reports
only aggregate counters and feedback timing; the generated invitation URL is
never emitted.
Both commands print JSON without paths, headers or response bodies. Each result counts CDP
request starts from navigation until the home marker is visible and timing is
read after the load event, including redirects; completed encoded
bytes sum `Network.loadingFinished.encodedDataLength` in that interval.
Unfinished requests, including long-lived streams, are counted separately and
their bytes are unavailable. Redirect bytes may be incomplete; HTTP responses
with status 400 or higher and transport failures are counted separately. HTTP
error details contain only the status, an allowlisted resource type and a
`favicon` or `other` classification; URLs and query strings are never emitted.
Redirects and CDP cache-served events are also reported. For interactions,
`completedEncodedBytes` counts completed response bytes; it does not include
request bodies. `requestBodyBytes` reports known request body lengths
separately. Timings are Navigation Timing milliseconds from navigation start
to DOMContentLoaded and load. This is one unthrottled run,
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
| Items list, create, edit | `scripts/playwright/ui.spec.mjs`; #413–#416 | Empty list cold/warm navigation, one create and edit, plus empty-title browser-validation timing measured below. Fragment/SSE bytes, populated, server validation/error, permission and no-JS performance states unmeasured. |
| Clients list and detail | `scripts/playwright/clients-list.spec.mjs`; #415 | Empty list cold/warm navigation measured below. SSE refresh and validation behavior tested; detail, populated, pending, failure, permission, and no-JS performance measures absent. |
| Files list and upload | `scripts/playwright/files.spec.mjs`; #417 | Empty list cold/warm navigation and one synthetic 256 KiB upload body size, response bytes, and success feedback measured below. Download wire bytes and upload progress duration, failure, permission and no-JS performance states remain unmeasured. |
| Members and invitations | `scripts/playwright/members.spec.mjs`, `invitations.spec.mjs`; #419, #420 | Cold/warm navigation and one synthetic invitation creation measured below. Member changes, acceptance, pending, validation, failure, permission and no-JS performance measures absent. |
| General/email settings, tokens/SCIM, webhooks/deliveries, audit, import/export | [Administration evidence](ui-measurement-admin.md); #422, #423, #424, #425, #426 | Cold/warm navigation and selected audit, import-preview and export interactions measured. Detailed states and limits are in linked evidence. |
| Account, security, sessions, notifications | `scripts/playwright/settings.spec.mjs`, `notifications.spec.mjs`; #427, #429 | Cold/warm navigation measured. Notification SSE connection state measured; delivery, replay, reconnect, mutation timings and other states remain unmeasured. See [identity evidence](ui-measurement-identity.md). |
| Setup, login, MFA, recovery | `scripts/playwright/ui.spec.mjs`, `password-reset.spec.mjs`; #430 | Cold/warm navigation and invalid-login/password-recovery feedback measured. Valid login, MFA, setup submission, validation, other failures, pending and no-JS timings remain unmeasured. See [identity evidence](ui-measurement-identity.md). |
| Public share and consent | `scripts/playwright/ui.spec.mjs`; #431 | Cold/warm unavailable-share navigation measured. Valid/expired/revoked share and consent navigation/interactions remain unmeasured. See [identity evidence](ui-measurement-identity.md). |
| Loading, empty, pending, validation, error, permission, no-JS across routes | Route-specific tests above | Selected admin state observations exist; other states and cross-screen loading feedback durations are unmeasured. |

No per-screen performance budgets or fixes are proposed. Asset raw/gzip sizes
are a shared inventory, not attribution to each screen. Current navigation
measurement counts all requests in the interval; it does not identify initiator,
script ownership, or whether repeat requests are unnecessary. HTMX, SSE,
upload/download wire bytes and loading feedback require separately bounded
interaction measurements. See route owners above for follow-up; no behavior
change is claimed.

### Members and invitation observation

`UI_MEASURE=members ./scripts/playwright-smoke.sh` uses the isolated owner and
one synthetic member fixture. It records cold/warm Members and Invitations
navigation, then creates one invitation for `ui-measurement-invite@example.test`.
The generated invitation URL remains in the browser only. The fixture is
discarded at exit. Member changes, invitation acceptance, and error/permission
states are not measured in this run.

On 2026-11-18T03:03:04Z, the isolated run completed on commit
`c54314cedf6ea614ffb7efc1501f71a598d84eb7`, Darwin 27.2.0 arm64, Node
26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1:

```sh
PLAYWRIGHT_PORT=59211 UI_MEASURE=members ./scripts/playwright-smoke.sh
```

The fixture had one owner, one workspace, one synthetic member and no
invitations. Browser conditions were 1280×900, device scale 1, `en-US`, light
theme, reduced motion and unthrottled loopback. Cold cleared HTTP cache after
sign-in; warm repeated each page in the same context. This is one sample, not a
budget. Request counts span navigation to load and visible heading; bytes sum
completed CDP encoded bytes. Timings are Navigation Timing milliseconds.

| Screen or interaction | Cache | Requests | Completed encoded bytes | DOMContentLoaded ms | Load or feedback ms | HTTP errors |
|---|---|---:|---:|---:|---:|---|
| Members | cold | 6 | 124321 | 26.8 | 28.8 | One favicon 404 |
| Members | warm | 5 | 123809 | 15.0 | 16.0 | None |
| Invitations | cold | 6 | 116898 | 15.8 | 16.0 | One favicon 404 |
| Invitations | warm | 5 | 116386 | 15.6 | 15.7 | None |
| Create invitation | — | 5 | 117548 | — | 83.2 | None |

Invitation creation returned HTTP 200. Feedback timing includes Playwright
automation overhead. The browser-only invitation link was not emitted. Member
changes, invitation acceptance, populated invitation history, failure,
permission and no-JavaScript performance states remain unmeasured.

### Members and invitations current rerun

On 2026-11-18T03:08:18Z, commit
`9fc03f2c6312086167949344bd97d91c837a3a20`, Darwin 27.2.0 arm64,
Go 1.27.4, Node 26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1:

```sh
PLAYWRIGHT_PORT=59141 UI_MEASURE=members ./scripts/playwright-smoke.sh
```

One unthrottled run used a fresh SQLite fixture with one owner, one workspace,
one synthetic member and no invitations. Browser conditions were 1280×900,
device scale 1, `en-US`, light theme, reduced motion and blocked service
workers. Cold cleared HTTP cache; warm repeated the route in the same context.
Counts span navigation through load and a visible route marker. Cold pages each
reported one favicon 404; there were no other HTTP errors, redirects, transport
failures or unfinished requests.

| Screen / interaction | Cache | Requests | Completed encoded bytes | DOMContentLoaded ms | Load / feedback ms |
|---|---|---:|---:|---:|---:|
| Members | Cold | 6 | 124321 | 28.2 | 32.7 |
| Members | Warm | 5 | 123809 | 13.0 | 16.9 |
| Invitations | Cold | 6 | 116898 | 20.1 | 20.4 |
| Invitations | Warm | 5 | 116386 | 15.1 | 15.4 |
| Invitation creation | — | 5 | 117548 | — | 77.4 |

Invitation creation returned 200. Feedback timing includes Playwright
automation overhead; the browser-only invitation URL was not emitted. These
single observations do not establish distributions or budgets.

The previous core rerun used a fresh isolated fixture. Upload completed, but
that runner version did not capture request body size. The prior failure came
from a role locator matching both the file input and submit button.

On 2026-11-18T03:12:21.005Z, commit
`e4b317dc791f519dbb9d1b24a16f00a1ce94e7ea`, Darwin 27.2.0 arm64, Node
26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1:

```sh
PLAYWRIGHT_PORT=59434 UI_MEASURE=core ./scripts/playwright-smoke.sh
```

The isolated fixture and browser conditions match the core run above. Item
creation returned 200 with 7,245 completed encoded bytes and 46.2 ms feedback;
empty-title validation took 42.6 ms and sent no request. The synthetic 256 KiB
file upload returned 303, with 7 requests, 1 redirect, 128,354 completed
encoded response bytes, zero HTTP errors or transport failures, and 89.6 ms
feedback. Request body size was unavailable in this run. Interaction durations
include Playwright overhead. This single successful upload does not measure
failure, permission, progress timing, download bytes or no-JavaScript fallback.

### Upload request-body observation

On 2026-11-18T03:22:34.617Z, commit
`e4302a4e831dd384bd65aa199530a7784c1f5ef2`, Darwin 27.2.0 arm64, Node
26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1:

```sh
PLAYWRIGHT_PORT=59593 UI_MEASURE=core ./scripts/playwright-smoke.sh
```

The fresh isolated SQLite fixture and browser conditions match the preceding
core run. Items, Clients and Files list request counts, completed response
bytes, and timings repeated the earlier values. Cold list pages each reported
one favicon 404; live Items and Clients pages each had one unfinished SSE
request. Item creation returned 200 with 123 request-body bytes and 7,245
completed response bytes; feedback was 49.5 ms. Empty-title browser validation
took 59.7 ms and sent no request. The synthetic 256 KiB upload returned 303
with 262,477 request-body bytes, 128,354 completed response bytes, 7 requests,
1 redirect and 83.2 ms feedback. No HTTP or transport errors occurred.

Request-body bytes exclude HTTP headers and framing. Feedback includes
Playwright automation overhead. This single run is not a distribution or
budget. Upload failure/progress, download bytes, permission and no-JavaScript
states remain unmeasured.

### Core list and Item interaction observation

On 2026-11-18T03:26:19.521Z, commit
`bb93d9ea53d0596a9f98ee95db6f7601b6cfbd7c`, Darwin 27.2.0 arm64, Go
1.27.4, Node 26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1, the
isolated core harness completed:

```sh
PLAYWRIGHT_PORT=59435 UI_MEASURE=core ./scripts/playwright-smoke.sh
```

The fresh SQLite fixture had one owner, one workspace and no Items, Clients
or Files. Browser conditions were 1280×900, device scale 1, `en-US`, light
theme, reduced motion, blocked service workers and unthrottled loopback. Cold
cleared HTTP cache; warm repeated each route in the same context. Requests
span navigation to load and a visible route marker. Bytes are completed CDP
encoded response bytes. Items and Clients each had one unfinished SSE request;
it is counted as unfinished, not a completed page response. Each cold route
reported one favicon 404. There were no other HTTP errors, redirects or
transport failures except the upload redirect.

| Screen / interaction | Cache | Requests | Completed encoded bytes | Request body bytes | DOMContentLoaded ms | Load / feedback ms |
|---|---|---:|---:|---:|---:|---:|
| Items list | Cold | 10 | 167739 | 0 | 20.7 | 24.7 |
| Items list | Warm | 9 | 167227 | 0 | 15.0 | 16.5 |
| Clients list | Cold | 10 | 170170 | 0 | 16.8 | 19.5 |
| Clients list | Warm | 9 | 169658 | 0 | 14.5 | 16.8 |
| Files list | Cold | 6 | 119061 | 0 | 15.9 | 17.3 |
| Files list | Warm | 5 | 118549 | 0 | 13.3 | 14.6 |
| Item creation | — | 1 | 7245 | 123 | — | 87.0 |
| Empty-title browser validation | — | 0 | unavailable | 0 | — | 83.7 |
| Synthetic 256 KiB upload | — | 7 | 128354 | 262477 | — | 141.7 |

Item creation returned 200; upload returned 303 with one redirect. No captured
request bodies had unknown sizes. Validation used the native `required`
constraint and sent no request. Interaction feedback includes Playwright
automation overhead. One observation does not establish a distribution or
budget. Client detail, download wire bytes, populated records,
server-side validation, upload progress/failure, permission and no-JavaScript
performance states remain unmeasured. The open SSE requests explain why list
request counts exceed completed responses. Request initiators and unnecessary
requests remain unattributed.

### Item edit interaction rerun

On 2026-11-18T03:34:53Z, the isolated core runner on application commit
`5c37624057ba95c95627645ceda008d24ac52919` used a fresh SQLite fixture with
one owner and workspace and no Items, Clients or Files. The measurement runner
included the synthetic edit capture added for this run. Browser conditions
were Darwin 27.2.0 arm64, Chrome 157.0.8103.79, Node 26.10.0, Playwright
1.64.1, 1280×900, device scale 1, `en-US`, light theme, reduced motion and
unthrottled loopback. This was one run, not a distribution or budget.

Item creation returned 200 with one request, 7,245 completed encoded response
bytes, 123 request-body bytes and 52.6 ms feedback. Editing that Item's title
returned 200 with one request, 7,273 completed encoded response bytes, 119
request-body bytes and 42.2 ms feedback. Empty-title native browser validation
sent no request and showed feedback in 40.3 ms. Interaction timings include
Playwright automation overhead. The run repeated earlier list and upload
observations; cold navigations again each reported one favicon 404, and the
Items and Clients list SSE requests remained unfinished. Server-side validation,
edit failure, permission and no-JavaScript performance states remain unmeasured.

## Current rerun on issue branch

On 2026-11-18T02:35:36Z, commit
`132086464fa822f5b2510de4eb061ec3317d5fc5`, Darwin 27.2.0 arm64,
Go 1.27.4, Node 26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1,
the core harness completed:

```sh
PLAYWRIGHT_PORT=59101 UI_MEASURE=core ./scripts/playwright-smoke.sh
```

It used the fresh isolated SQLite fixture and browser conditions above. This
single unthrottled sample repeats the previous empty-list baseline. Items,
Clients and Files cold/warm requests, bytes and timings were unchanged from the
previous recorded sample. The three cold navigations again reported one favicon
404; Items and Clients each had one unfinished SSE request. Synthetic Item
creation returned 200 with 7245 completed encoded bytes and 125.9 ms feedback;
empty-title native validation sent no request and showed feedback in 66.6 ms.
These are observations, not budgets or distributions.

On 2026-11-18T02:07:21.220Z, commit
`4492d2c86536e6f528c7bb9a73287d568ee081ff`, Darwin 27.2.0 arm64,
Go 1.27.4, Node 26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1:

```sh
PLAYWRIGHT_PORT=58971 UI_MEASURE=core ./scripts/playwright-smoke.sh
```

One unthrottled loopback run used a fresh SQLite fixture with one synthetic
owner, one workspace and zero Items, Clients and Files. Browser conditions
were 1280×900, device scale 1, `en-US`, light theme, reduced motion and blocked
service workers. Cold cleared HTTP cache before each navigation; warm repeated
the route in the same context. Counts span navigation through load and a visible
route marker. All three live list pages had one unfinished SSE request, counted
but excluded from completed bytes. Cold pages each reported the favicon 404;
there were no other errors, redirects or transport failures.

| Screen / interaction | Cache | Requests | Completed encoded bytes | DOMContentLoaded ms | Load / feedback ms |
|---|---|---:|---:|---:|---:|
| Items list | Cold | 10 | 167739 | 19.1 | 20.7 |
| Items list | Warm | 9 | 167227 | 15.1 | 16.5 |
| Clients list | Cold | 10 | 170170 | 15.7 | 17.9 |
| Clients list | Warm | 9 | 169658 | 25.4 | 29.2 |
| Files list | Cold | 6 | 119061 | 15.0 | 16.4 |
| Files list | Warm | 5 | 118549 | 14.0 | 15.3 |
| Item create | — | 1 | 7245 | — | 58.8 |
| Empty-title browser validation | — | 0 captured | unavailable | — | 104.6 |

Creation returned 200. Empty-title validation used the browser's native
`required` constraint, so it sent no request. Interaction durations use Node's
monotonic clock and include Playwright automation overhead. These single
observations do not establish distributions or budgets. The runner and results
do not measure populated states, Item edit, server validation, client detail,
download transfer, feedback under failure, permission/no-JavaScript
states, asset initiators or whether repeated requests are unnecessary.

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

On 2026-11-18T01:35:54.846Z, commit
`d47ad87eab45db60315310a662e3d57d46a91e4e`, Darwin 27.2.0 arm64,
Go 1.27.4, Node 26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1:

```sh
PLAYWRIGHT_PORT=58311 UI_MEASURE=1 ./scripts/playwright-smoke.sh
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

The latest rerun on this issue branch used the same fixture and conditions:

| Cache | Requests | Completed encoded bytes | DOMContentLoaded ms | Load ms |
|---|---:|---:|---:|---:|
| Cold | 5 | 98293 | 10.6 | 12.0 |
| Warm | 4 | 97781 | 13.4 | 14.7 |

The cold run again recorded one favicon 404; warm had no HTTP errors. There
were no redirects, transport failures, cache-served events or unfinished
requests. This remains one sample per cache condition.

This run measures workspace-home navigation only. It does not measure other
routes, screen interactions, populated states, loading duration, compression
over HTTP or per-resource attribution. The cold favicon 404 remains unassigned
to a fix ticket.

### Current core rerun

On 2026-11-18T03:38:18.415Z, commit
`48907564e779597e849c8fdcf2df8d4392ebd0a6`, Darwin 27.2.0 arm64, Go
1.27.4, Node 26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1:

```sh
PLAYWRIGHT_PORT=59231 UI_MEASURE=core ./scripts/playwright-smoke.sh
```

One unthrottled run used a fresh SQLite fixture with one owner, one workspace,
and zero Items, Clients or Files. Browser conditions were 1280×900, device
scale 1, `en-US`, light theme, reduced motion and loopback. Cold cleared HTTP
cache after sign-in; warm repeated each route in the same context. Navigation
counts span request starts to load and a visible route marker; bytes are
completed CDP encoded response bytes. Items and Clients each had one unfinished
SSE request. Each cold page reported one favicon 404; no other errors,
redirects or transport failures occurred.

| Screen / interaction | Cache | Requests | Completed encoded bytes | DOMContentLoaded ms | Load / feedback ms |
|---|---|---:|---:|---:|---:|
| Items list | Cold | 10 | 167739 | 17.8 | 19.4 |
| Items list | Warm | 9 | 167227 | 14.3 | 15.9 |
| Clients list | Cold | 10 | 170170 | 16.3 | 18.6 |
| Clients list | Warm | 9 | 169658 | 16.7 | 18.8 |
| Files list | Cold | 6 | 119061 | 14.6 | 15.9 |
| Files list | Warm | 5 | 118549 | 14.1 | 15.6 |
| Item create | — | 1 | 7245 | — | 56.0 |
| Item title edit | — | 1 | 7273 | — | 57.4 |
| Empty-title browser validation | — | 0 | unavailable | — | 57.7 |
| Synthetic 256 KiB upload | — | 7 | 128354 | — | 88.9 |

Item create and edit returned 200 with 123 and 119 request-body bytes. Empty
title validation sent no request. Upload returned 303 with 262477 request-body
bytes, including multipart overhead. Interaction feedback timings include
Playwright overhead. These are single observations, not distributions or
budgets. Populated lists, Client detail, server validation, failure and
permission states, download wire bytes, no-JavaScript interactions, and
per-resource script initiators remain unmeasured.
