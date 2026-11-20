# Administration and data measurement evidence

Issue #448 covers general and email settings, API tokens, SCIM provisioning,
webhooks and deliveries, audit history, import preview, and export. The common
[measurement method](ui-measurement.md) applies. The runs below complete the
bounded screen and state measurements for this issue. They do not establish
budgets or claim performance fixes.

The separate `sh scripts/ui-measurement-admin.sh` command creates a temporary
SQLite fixture and runs `scripts/ui-measurement-admin.mjs` (retired). Install dependencies
with `npm ci` and use the Chrome prerequisites in the common method first.
The current harness builds the binaries, then starts bootstrap and server with
an allowlisted environment through `scripts/ui-measurement-admin-env.sh`: a
temporary SQLite database and local file store, a test-only webhook key,
loopback HTTP, and `OTEL_SDK_DISABLED=true`. Caller storage, telemetry and mail
provider settings are excluded. `node --test scripts/ui-measurement-admin-env.test.mjs`
checks this boundary without starting the server or collecting measurements.
It records cold and warm navigation to eight settings pages, with the same CDP
network definitions as #446. The later webhook-deliveries page is also measured.
Each cold navigation clears the HTTP cache after
sign-in; warm repeats the same page. A single context is reused. Each interval
ends after the load event and visible main heading. The audit lazy request may
overlap that interval and must not be interpreted as a separate fragment result.
The script creates one synthetic webhook with a fixed test-only encryption key
after the initial navigation runs. It does not trigger a delivery. Its secret
appears once in the browser but is never recorded. Deliveries navigation is
measured cold and warm with an empty delivery list. The audit fragment is a
separate warm `fetch` with the same header as `forms.js`, from an audit page
loaded with `?all=1`; it is not a measurement of the automatic fetch or DOM
swap. The harness also observes the actual automatic audit request and DOM
replacement, with a best-effort loading-indicator observation. The import
preview submits one synthetic CSV row without confirming it.
The export download is saved only in Playwright's temporary browser directory.

The fragment interval runs from immediately before `fetch` until its response
body is consumed. The preview interval runs from before the submit click until
the preview is visible after navigation. The download interval runs from before
the click until Playwright provides the completed temporary file. These
interaction durations use Node's monotonic clock; they include automation
overhead. Their CDP counters use the common observer, not isolated per-request
accounting. Download `decodedBytes` is the local file size, not wire transfer;
download transfer bytes are unavailable. Chrome may report a download as a
transport failure because navigation becomes a download; do not interpret that
counter alone as a failed download. Loading feedback outside the audit indicator,
SSE and per-request script attribution are not captured.

The output contains aggregate counters, safe HTTP error classifications and
environment metadata, never URLs, response bodies, credentials or headers.
Errors from the browser script are suppressed because they can contain page
contents or URLs. A nonzero exit therefore requires local investigation; do not
share unsanitized diagnostics.
Run only against the isolated synthetic fixture. Save the exact command, commit,
output and fixture conditions when results are collected.

| Screen or interaction | Prepared coverage | Unverified |
|---|---|---|
| General and email settings | Cold/warm navigation; viewer GET/POST denial; Spanish mobile scan | Save, validation, failure, configured provider states |
| Tokens and SCIM | Cold/warm navigation; viewer GET/POST denial; Spanish mobile scan | Create, revoke and one-time secret states |
| Webhooks and deliveries | Cold/warm navigation; empty, pending, delivered, failed; viewer denial; Spanish mobile scan | Other delivery failures and retry timing |
| Audit | Cold/warm navigation; automatic fetch and DOM swap; synthetic warm fetch; injected failure; no-JavaScript fallback; anonymous redirect; viewer denial; Spanish mobile scan | Isolated automatic-fragment duration and other failures |
| Import | Cold/warm navigation; valid and invalid one-row preview with and without JavaScript; viewer denial; Spanish mobile scan | Other validation and failure states; successful confirmation |
| Export | Cold/warm navigation; download and local decoded file size; viewer denial; Spanish mobile scan | Download wire bytes and failure |

Viewer GET and selected POST responses are recorded below. A Spanish mobile
automated scan is recorded below; keyboard behavior and no-JavaScript flows
outside the two observed cases remain unverified.
Do not capture one-time token or webhook secrets, SMTP credentials, CSRF fields,
export bodies, endpoint URLs, traces or screenshots. The harness cannot attribute
per-request script initiators, measure download wire bytes, or isolate overlapping
audit navigation requests. Those metrics remain unavailable; no fixes or budgets
are proposed from these measurements.

## Isolated timing run

On 2026-11-17T18:46:01.823Z, commit
`1c2ff7f59d106988976daf1370e52463f60d0896`, Darwin 27.2.0 arm64, Go 1.27.4,
Node 26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1:

```sh
sh scripts/ui-measurement-admin.sh > /tmp/ui-448-current-measurement.json
```

One unthrottled loopback run used a fresh SQLite fixture with one owner and one
workspace. It began with zero items and added one webhook with zero deliveries.
Viewport was 1280×900 at device scale 1, locale `en-US`, light theme, and reduced
motion. Cold navigation cleared the HTTP cache after sign-in; warm navigation
repeated the same page in one browser context. Each interval ended after load and
a visible heading. Values below are one observation, not a distribution.

| Page | Cache | Requests | Completed encoded bytes | DOMContentLoaded ms | Load ms | HTTP errors |
|---|---|---:|---:|---:|---:|---|
| General | cold | 5 | 99752 | 12.8 | 16.3 | one favicon 404 |
| General | warm | 4 | 99240 | 13.2 | 15.5 | none |
| Email | cold | 5 | 102276 | 10.6 | 13.6 | one favicon 404 |
| Email | warm | 4 | 101764 | 10.0 | 13.3 | none |
| Tokens | cold | 7 | 116325 | 19.7 | 19.9 | one favicon 404 |
| Tokens | warm | 6 | 115813 | 14.3 | 14.5 | none |
| SCIM provisioning | cold | 5 | 99554 | 9.6 | 11.2 | one favicon 404 |
| SCIM provisioning | warm | 4 | 99042 | 9.4 | 11.0 | none |
| Webhooks | cold | 5 | 79134 | 17.2 | 17.4 | one favicon 404 |
| Webhooks | warm | 4 | 78622 | 11.6 | 11.9 | none |
| Audit | cold | 6 | 100144 | 10.6 | 11.8 | one favicon 404 |
| Audit | warm | 5 | 99632 | 9.6 | 10.7 | none |
| Import | cold | 5 | 98811 | 65.4 | 66.8 | one favicon 404 |
| Import | warm | 4 | 98299 | 9.6 | 11.1 | none |
| Export | cold | 6 | 115212 | 13.5 | 13.6 | one favicon 404 |
| Export | warm | 5 | 114700 | 11.3 | 11.3 | none |
| Webhook deliveries | cold | 5 | 77523 | 12.7 | 12.8 | one favicon 404 |
| Webhook deliveries | warm | 4 | 77011 | 9.9 | 10.8 | none |

Every navigation had zero redirects, transport failures, and unfinished
requests. Each cold navigation recorded one favicon 404; warm navigations had no
HTTP errors. The token screen reported one cache-served event in both runs, so
its cold label does not prove every resource came from the network. No duplicate
allowlisted assets appeared in the observed intervals; this does not establish
that any asset is necessary or unnecessary.

| Interaction | Requests | Completed encoded bytes | Elapsed ms | Other |
|---|---:|---:|---:|---|
| Automatic audit fragment and DOM swap | unavailable | unavailable | 29.9 from navigation start | loading indicator observed; header 534 bytes, body 1500 bytes |
| Synthetic audit fragment fetch | 1 | 2034 | 5.8 | warm; response body consumed |
| CSV import preview | 4 | 100043 | 81.1 | warm; one valid synthetic row; not confirmed |
| Export download | 1 | unavailable | 140.7 | local decoded file 2057 bytes; wire bytes unavailable |

Interaction durations use Node's monotonic clock and include automation overhead.
The automatic audit measurement is from navigation start, and its request
overlaps navigation; it is not an isolated fragment duration. The export
completed despite Chrome reporting one transport failure; its transfer bytes
remain unavailable. No duplicate or unnecessary request conclusion follows from
these counts.

On the same commit, the isolated state command completed successfully:

```sh
UI_ADMIN_STATES=1 sh scripts/ui-measurement-admin.sh > /tmp/ui-448-current-states.json
node --test scripts/ui-measurement-admin-attribution.test.mjs scripts/ui-measurement-admin-env.test.mjs
```

The owner observed one pending delivery row, then one delivered and one failed
delivery from loopback 204 and 503 receivers. Audit fragment HTTP 503 displayed
failure feedback; the no-JavaScript audit fallback rendered its table. Invalid
CSV showed validation feedback with and without JavaScript. An anonymous audit
request reached login. A viewer received 200 for general and tokens, and 403 for
email, SCIM, webhooks, audit, import and export. Viewer POSTs to general, email,
SCIM and webhooks returned 403. Eight Spanish mobile system-dark screens had no
axe violations and no horizontal overflow. These are observations from one
synthetic fixture; token/SCIM create, revoke, one-time display, configured email
providers, successful import confirmation, and export failure remain unverified.

## State and attribution checks

The separate command below creates a fresh fixture and produces state and
request-class counts without timing metrics.

```sh
UI_ADMIN_STATES=1 sh scripts/ui-measurement-admin.sh > /tmp/ui-448-viewer-states.json
node --test scripts/ui-measurement-admin-attribution.test.mjs scripts/ui-measurement-admin-env.test.mjs
```

The browser run signs in as the synthetic owner and counts allowlisted
asset names per settings navigation until load and a visible heading. Other
same-origin requests are grouped by resource type; external requests are
counted without their URLs. Repeated counts identify only repeated allowlisted
assets in that interval, not whether they are unnecessary. Requests after the
interval, unlisted assets, individual paths, bytes and script initiators are
not attributed. Query strings, headers, bodies, IDs and secret values are never
reported. The attribution unit test checks that private paths and query values
do not reach output.

The state checks inject an HTTP 503 for the audit fragment, submit an
invalid synthetic CSV preview, follow the audit fallback with JavaScript off,
and inspect the final status and login redirect for an anonymous audit request.
They create one synthetic webhook and item; a temporary loopback-only receiver returns HTTP 503
for webhook delivery. The output reports delivery row and pending counts once;
the worker can change status asynchronously, so a missing pending observation
is not proof that pending never occurred. The receiver does not record payloads
or headers. A second loopback endpoint returns HTTP 204. A separate context
accepts a synthetic viewer invitation and records GET status for eight settings
pages and POST status for four. The invitation link and credentials are not
reported. The worker is polled for a bounded period; a missing outcome is
reported as unobserved, not as zero. Further state coverage remains below.

On 2026-11-16 before 17:55:04 UTC, commit
`a646ef788a6c18b9601b61d85e3bbc877ce0407f` ran the command above successfully.
Go test and race processes were
active concurrently; no timing metrics were collected. The owner fixture used
one workspace, a fresh SQLite database and a loopback-only webhook receiver.
The audit fragment's injected HTTP 503 produced visible failure feedback; an
invalid CSV produced visible validation feedback. The deliveries page showed
one row and one pending status. The no-JavaScript audit fallback rendered its
table. An anonymous audit request reached login with final HTTP 200; that final
status is the login page response, not the initial redirect status.

| Viewer GET | Final HTTP status |
|---|---:|
| General, tokens | 200 |
| Email, SCIM provisioning, webhooks, audit, import, export | 403 |

For the owner navigation interval, all eight screens requested one document,
`app.css` and `forms.js` each. Email, tokens, webhooks and export also requested
`app.js`; import requested `import.css`; audit recorded one fetch. Each listed
asset appeared once per screen. The earlier run at
`45e805a5b0c531a85bd448ba14fad5241ee17415` reported the same counts and the
same five state outcomes. No conclusion about requests outside
the observation interval or whether any script is unnecessary follows from these
counts. Both runs completed and their temporary servers and receivers stopped.

On 2026-11-16 before 18:06:00 UTC, commit
`1df4a254238fe89c89564b9903d54a0ca6b45617` ran
`UI_ADMIN_STATES=1 sh scripts/ui-measurement-admin.sh > /tmp/ui-448-final-states.json`
with Go test and race processes active concurrently. The isolated receiver returned
503 for one webhook and 204 for the other. The deliveries page showed pending,
then failed for the former and delivered for the latter. The 503 result follows
the retry exhaustion path in `evaluateDeliveryOutcome`; this observation did
not independently record attempt counts or retry intervals. Viewer POSTs with a
valid session CSRF value to general, email, provisioning and webhooks returned
403. Invalid CSV feedback also appeared with JavaScript disabled. The server,
browser and loopback receiver stopped after the run. No timings were recorded.

At 390×844, device scale 1, Chrome 157.0.8103.78, browser locale `es-ES`,
Spanish language cookie and dark OS color preference, all eight settings pages
reported `lang=es`, `data-theme=system`, and no document-level horizontal
overflow. Axe-core 4.14.0 scanned WCAG 2 A/AA and 2.1 AA tags. Seven pages
reported no violations. **repo — action: [#425](https://github.com/dynamiscode/dynamis-code-apps-template/issues/425):**
the audit page reported one serious `scrollable-region-focusable` violation;
the affected element matched `#audit-table .table-wrap`. The implementation
now gives that region keyboard focus and an accessible name. This automated
finding is not proof of complete accessibility or WCAG conformance. The scan does
not cover delivery history, error states, keyboard operation, screen readers,
other widths, explicit light/dark account preferences, zoom or all locales.

The first two expanded attempts did not yield results because inline axe
injection failed under the page's script policy. The successful run used
Playwright's initialization script; no product policy was changed. Remaining
work includes viewer mutations beyond the four endpoints, non-JavaScript flows
beyond audit and invalid import, additional error and provider states, request
initiators and attribution outside the observed interval, export wire bytes,
and other screen/locale/theme combinations. The #425 focusability finding is
addressed; current English and Spanish axe results are in
[accessibility evidence](accessibility.md).

## Earlier isolated run

On 2026-11-16T17:06:04.017Z, commit
`763a333db4afb6c1f6dda84acfb33e42d2a6854e`, Darwin 27.2.0 arm64,
Go 1.27.4, Node 26.10.0, Chrome 157.0.8103.78 and Playwright 1.64.1:

Dependencies were installed earlier with `npm ci`. The measurement command was:

```sh
sh scripts/ui-measurement-admin.sh > /tmp/ui-448-isolated-measurement.json
```

This run replaces the provisional run at `ef822879b8ffb51d6ab91d9f533c830a07f665f7`,
which inherited caller application configuration and did not isolate local file
storage. Independent draft PR submission checks were authorized to run
concurrently; CPU and I/O contention are possible, so these timings are not
claimed to be uncontended.

One unthrottled loopback run used a fresh temporary SQLite database: one owner,
one workspace, no items and initially no webhooks. It added one synthetic
webhook with no deliveries, then previewed one CSV row without importing it.
Viewport was 1280×900 at device scale 1, browser locale `en-US`, default account
locale, light theme and reduced motion. The browser used one context with
service workers blocked. Each cold run cleared the HTTP cache after sign-in;
each warm run immediately repeated that page. The token page reported one CDP
cache event even after clearing, so the cold label does not prove every resource
was served over the network. Fixture and browser were removed at exit.

The following values are one observation, not a distribution. Navigation
requests count CDP request starts until load and the visible heading; bytes sum
completed `Network.loadingFinished.encodedDataLength` events in that interval.
Timings are Navigation Timing milliseconds from navigation start to
DOMContentLoaded and load, rounded to one decimal. They do not measure the
rendered result of an audit fragment.

| Page | Cache | Requests | Completed encoded bytes | DOMContentLoaded ms | Load ms |
|---|---|---:|---:|---:|---:|
| General | cold | 4 | 59055 | 18.6 | 21.8 |
| General | warm | 3 | 58543 | 12.2 | 14.3 |
| Email | cold | 5 | 76653 | 23.0 | 24.1 |
| Email | warm | 4 | 76141 | 16.5 | 17.5 |
| Tokens | cold | 6 | 73975 | 18.4 | 26.8 |
| Tokens | warm | 5 | 73463 | 15.6 | 15.9 |
| SCIM provisioning | cold | 4 | 58875 | 13.1 | 14.2 |
| SCIM provisioning | warm | 3 | 58363 | 12.5 | 12.5 |
| Webhooks | cold | 5 | 74321 | 21.0 | 21.2 |
| Webhooks | warm | 4 | 73809 | 11.9 | 13.6 |
| Audit | cold | 5 | 59775 | 12.8 | 13.8 |
| Audit | warm | 4 | 59263 | 15.3 | 16.3 |
| Import | cold | 5 | 59439 | 15.7 | 16.8 |
| Import | warm | 4 | 58927 | 11.4 | 12.5 |
| Export | cold | 5 | 72744 | 13.8 | 13.9 |
| Export | warm | 4 | 72232 | 14.9 | 15.0 |
| Webhook deliveries | cold | 5 | 72710 | 22.1 | 22.3 |
| Webhook deliveries | warm | 4 | 72198 | 13.3 | 13.3 |

Every cold navigation recorded one favicon HTTP 404; warm navigations recorded
no HTTP errors. All navigations had zero redirects, transport failures and
unfinished requests. The token page reported one cache event in each run; the
other pages reported none. Per-resource request attribution was not collected,
so duplicate and unnecessary requests cannot be assessed beyond the repeated
missing favicon. Investigate the favicon in the owning shared-shell work; no fix
is claimed here.

| Interaction | Requests | Completed encoded bytes | Elapsed ms | Other |
|---|---:|---:|---:|---|
| Audit fragment fetch | 1 | 1994 | 5.0 | Synthetic warm fetch, not automatic swap |
| CSV import preview | 4 | 60436 | 85.3 | Warm; one synthetic row; no confirmation |
| Export download | 1 | unavailable | 118.1 | Warm; 2057 decoded file bytes; wire bytes unavailable |

For the actual automatic audit request, the warm page's DOM replacement was
observed at 45.3 ms from navigation start and the loading indicator was
observed. Playwright reported 534 response header bytes and 1460 response body
bytes for the fragment. These are Playwright request-size fields, not the CDP
completed-byte sum above. The loading observation does not measure duration or
prove accessibility of the feedback.

The three CDP-observed interactions in the table had no redirects, HTTP errors,
cache events or unfinished requests. Those counters were not recorded separately
for the automatic audit request. Chrome reported one transport failure for the download, while
Playwright returned a completed file; its cause is unverified. The other two
interactions reported no transport failures. The download observer reported
zero completed encoded bytes; that is not a measurement of its wire size.
`npm ci` warned that the installed Node version is below selenium-webdriver's
declared minimum; this harness uses Playwright, and the run completed. No
browser performance measurement was repeated to estimate variability.

## Current rerun on issue branch

On 2026-11-18T02:35:28Z, commit
`132086464fa822f5b2510de4eb061ec3317d5fc5`, Darwin 27.2.0 arm64,
Go 1.27.4, Node 26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1,
the isolated admin harness completed:

```sh
PLAYWRIGHT_PORT=59111 sh scripts/ui-measurement-admin.sh
```

All eight settings/data screens and webhook deliveries again produced cold and
warm navigation observations. The sample again showed one cold favicon 404
per navigation; tokens recorded one cache hit in both conditions. Automatic
audit swap and loading feedback appeared (35.5 ms); warm manual fragment fetch
was 2034 encoded bytes (4.5 ms), and one-row import preview was 100043 encoded
bytes (70.2 ms). Export completed to a 2057-byte local file in 84.3 ms; wire
bytes remained unavailable, and CDP reported one transport failure during the
download. Values are one unthrottled observation; they set no budget and do not
establish that a request is unnecessary.

## Earlier isolated navigation and interaction observation

On 2026-11-17T23:26:52.500Z, commit
`1c2ff7f59d106988976daf1370e52463f60d0896`, Darwin 27.2.0 arm64,
Go 1.27.4, Node 26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1:

```sh
PLAYWRIGHT_PORT=58961 sh scripts/ui-measurement-admin.sh
```

One unthrottled loopback run used a fresh SQLite fixture with one owner, one
workspace, zero Items, then one synthetic webhook with zero deliveries. Browser
conditions were 1280×900, device scale 1, `en-US`, light theme, reduced motion,
service workers blocked. Cold navigations cleared the HTTP cache after sign-in;
warm immediately repeated each route. Results are one sample. Counts and timing
boundaries match the [earlier run](#earlier-isolated-run). `DOMContentLoaded`
and `load` values are milliseconds from navigation start, rounded to one decimal.

| Screen | Cache | Requests | Completed encoded bytes | DOMContentLoaded ms | Load ms |
|---|---|---:|---:|---:|---:|
| General | cold | 5 | 99752 | 21.7 | 25.7 |
| General | warm | 4 | 99240 | 15.7 | 18.3 |
| Email | cold | 5 | 102276 | 12.7 | 19.9 |
| Email | warm | 4 | 101764 | 23.9 | 27.0 |
| Tokens | cold | 7 | 116325 | 29.1 | 29.3 |
| Tokens | warm | 6 | 115813 | 14.1 | 16.6 |
| SCIM provisioning | cold | 5 | 99554 | 14.7 | 16.2 |
| SCIM provisioning | warm | 4 | 99042 | 12.2 | 13.7 |
| Webhooks | cold | 5 | 79134 | 34.7 | 34.9 |
| Webhooks | warm | 4 | 78622 | 19.2 | 19.8 |
| Audit | cold | 6 | 100148 | 19.6 | 19.7 |
| Audit | warm | 5 | 99636 | 12.0 | 13.4 |
| Import | cold | 5 | 98811 | 22.1 | 24.0 |
| Import | warm | 4 | 98299 | 19.2 | 20.7 |
| Export | cold | 6 | 115212 | 21.2 | 22.5 |
| Export | warm | 5 | 114700 | 24.1 | 25.6 |
| Webhook deliveries | cold | 5 | 77523 | 15.5 | 15.6 |
| Webhook deliveries | warm | 4 | 77011 | 13.3 | 13.4 |

All cold navigations recorded one favicon HTTP 404; warm runs recorded no HTTP
errors. All navigations had zero redirects, transport failures and unfinished
requests. Tokens recorded one cache-served event in both cold and warm runs;
other navigations recorded none. The cache-clearing procedure therefore does
not prove every cold resource reached the network. Request counts include the
favicon. No per-resource initiator attribution was collected.

| Interaction | Requests | Completed encoded bytes | Elapsed ms | Observation |
|---|---:|---:|---:|---|
| Automatic audit fragment and DOM swap | unavailable | unavailable | 498.2 from navigation start | Includes navigation; loading observed; Playwright response sizes: 534 header bytes, 1505 body bytes |
| Separate warm audit fragment fetch | 1 | 2039 | 5.4 | Response body consumed |
| Warm CSV import preview | 4 | 100043 | 124.7 | One synthetic row; preview visible |
| Warm export download | 1 | unavailable | 313.9 | Completed local file: 2057 decoded bytes; wire bytes unavailable |

The automatic audit duration starts at navigation start, so it includes page
navigation and is not isolated fragment latency. CDP reported one transport
failure and zero completed encoded bytes for export even though Playwright
returned a completed file; interpret those CDP fields as unavailable for this
download. The audit fetch and import preview had no HTTP errors, redirects,
cache events or transport failures. Export had no HTTP errors, redirects or
cache events; its transport failure is described above. Download failure
classification and transfer bytes remain unverified.
The run observed a cold favicon 404 but did not assign a fix ticket. It proposes
no budget or implementation change.

## Current isolated navigation and interaction observation

On 2026-11-18T01:36:13.404Z, commit
`d47ad87eab45db60315310a662e3d57d46a91e4e`, Darwin 27.2.0 arm64,
Go 1.27.4, Node 26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1:

```sh
PLAYWRIGHT_PORT=58321 sh scripts/ui-measurement-admin.sh
```

One unthrottled loopback run used a fresh SQLite fixture with one owner, one
workspace, zero Items, then one synthetic webhook with zero deliveries. Browser
conditions and navigation/interaction boundaries match the
[earlier observation](#earlier-isolated-navigation-and-interaction-observation).
Results are one sample, not a distribution. Timing values are milliseconds
from Navigation Timing or the interaction boundaries described above, rounded
to one decimal.

| Screen | Cache | Requests | Completed encoded bytes | DOMContentLoaded ms | Load ms |
|---|---|---:|---:|---:|---:|
| General | cold | 5 | 99752 | 13.0 | 16.4 |
| General | warm | 4 | 99240 | 9.9 | 12.3 |
| Email | cold | 5 | 102276 | 9.7 | 12.8 |
| Email | warm | 4 | 101764 | 9.6 | 12.5 |
| Tokens | cold | 7 | 116325 | 16.3 | 16.5 |
| Tokens | warm | 6 | 115813 | 13.6 | 13.9 |
| SCIM provisioning | cold | 5 | 99554 | 9.3 | 10.6 |
| SCIM provisioning | warm | 4 | 99042 | 12.9 | 13.0 |
| Webhooks | cold | 5 | 79134 | 20.7 | 20.9 |
| Webhooks | warm | 4 | 78622 | 9.8 | 11.6 |
| Audit | cold | 6 | 100148 | 10.5 | 11.7 |
| Audit | warm | 5 | 99636 | 13.2 | 13.2 |
| Import | cold | 5 | 98811 | 13.7 | 15.4 |
| Import | warm | 4 | 98299 | 9.3 | 10.7 |
| Export | cold | 6 | 115212 | 13.3 | 13.4 |
| Export | warm | 5 | 114700 | 14.0 | 14.0 |
| Webhook deliveries | cold | 5 | 77523 | 14.5 | 14.6 |
| Webhook deliveries | warm | 4 | 77011 | 13.7 | 13.7 |

Every cold navigation recorded one favicon HTTP 404; warm runs recorded no
HTTP errors. All navigations had zero redirects, transport failures and
unfinished requests. Tokens recorded one cache-served event in both cold and
warm runs; other navigations recorded none. Cache clearing therefore does not
prove every cold resource reached the network. No per-resource attribution was
collected.

| Interaction | Requests | Completed encoded bytes | Elapsed ms | Observation |
|---|---:|---:|---:|---|
| Automatic audit fragment and DOM swap | unavailable | unavailable | 29.8 from navigation start | Includes navigation; loading observed; response sizes: 534 header bytes, 1505 body bytes |
| Separate warm audit fragment fetch | 1 | 2039 | 5.7 | Response body consumed |
| Warm CSV import preview | 4 | 100043 | 71.4 | One synthetic row; preview visible |
| Warm export download | 1 | unavailable | 74.7 | Completed local file: 2052 decoded bytes; wire bytes unavailable |

These are single observations. The automatic audit duration includes page
navigation. CDP reported one export transport failure and zero completed
encoded bytes although Playwright returned a completed file. Export transfer
bytes remain unavailable. The run proposes no budget or implementation change.

The latest issue-branch rerun repeated all eight settings navigations and the
webhook-deliveries page with the same fixture and boundaries:

| Screen | Cache | Requests | Completed encoded bytes | DOMContentLoaded ms | Load ms |
|---|---|---:|---:|---:|---:|
| General | cold | 5 | 99752 | 11.6 | 15.3 |
| General | warm | 4 | 99240 | 10.5 | 13.0 |
| Email | cold | 5 | 102276 | 16.7 | 19.6 |
| Email | warm | 4 | 101764 | 14.0 | 17.0 |
| Tokens | cold | 7 | 116325 | 18.7 | 23.5 |
| Tokens | warm | 6 | 115813 | 18.3 | 18.5 |
| SCIM provisioning | cold | 5 | 99554 | 10.1 | 11.7 |
| SCIM provisioning | warm | 4 | 99042 | 11.7 | 13.2 |
| Webhooks | cold | 5 | 79134 | 23.4 | 23.7 |
| Webhooks | warm | 4 | 78622 | 11.9 | 13.6 |
| Audit | cold | 6 | 100144 | 11.4 | 12.7 |
| Audit | warm | 5 | 99632 | 13.5 | 14.6 |
| Import | cold | 5 | 98811 | 12.4 | 14.0 |
| Import | warm | 4 | 98299 | 10.5 | 11.9 |
| Export | cold | 6 | 115212 | 12.9 | 13.0 |
| Export | warm | 5 | 114700 | 13.3 | 13.4 |
| Webhook deliveries | cold | 5 | 77523 | 13.2 | 13.2 |
| Webhook deliveries | warm | 4 | 77011 | 13.2 | 13.2 |

Each cold route recorded one favicon 404; warm routes had no HTTP errors.
Tokens reported one cache-served event in both conditions. Other navigation
intervals had no redirects, transport failures, cache-served events or
unfinished requests. Per-resource initiators remain unavailable.

| Interaction | Requests | Completed encoded bytes | Elapsed ms | Observation |
|---|---:|---:|---:|---|
| Automatic audit fragment and DOM swap | unavailable | unavailable | 47.8 from navigation start | Includes navigation; loading observed; response: 534 header bytes, 1500 body bytes |
| Separate warm audit fragment fetch | 1 | 2034 | 5.1 | Response body consumed |
| Warm CSV import preview | 4 | 100043 | 110.7 | One synthetic row; preview visible |
| Warm export download | 1 | unavailable | 124.2 | Completed local file: 2057 decoded bytes; wire bytes unavailable |

Export again reported one CDP transport failure and zero completed encoded
bytes despite a completed Playwright download. The cause and wire size remain
unverified. No budget or optimization is proposed from these single samples.
