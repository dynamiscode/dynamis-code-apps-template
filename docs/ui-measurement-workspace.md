# Workspace navigation measurements

Partial evidence for #447, using the [common method](ui-measurement.md). This
does not complete #447 or set a budget. The measurement script was added on
top of application commit `d438f9d39a7063c0292ce4731c81977ca8af9a81`.

**Isolation caveat:** A fixture isolation defect was found in upstream draft
PR #456 after these runs. Its scope and effect on these results are not yet
verified. Treat all recorded measurements as provisional until the upstream
fix is available and the affected scenarios are rerun with verified isolation.
The isolated reruns below supersede the corresponding provisional observations.

Run at 2026-11-16T16:09:46.085Z on Darwin 27.2.0 arm64, Chrome
157.0.8103.78, Node 26.10.0 and Playwright 1.64.1. Command:

```sh
npm ci
UI_MEASURE=workspace PLAYWRIGHT_PORT=58147 ./scripts/playwright-smoke.sh
```

The harness builds the embedded-asset server and creates a fresh SQLite
fixture: one owner, one workspace and zero Items, Clients, Files or invitations.
Members contains the owner. One unthrottled run used 1280×900, device scale 1,
English browser and default account locale, light theme, reduced motion and
blocked service workers. After sign-in, Chrome's HTTP cache was cleared before
each cold navigation; the following navigation to the same route is warm.
Concurrent workload was not recorded. A race suite was reported running during
follow-up work, so contention cannot be ruled out for this snapshot. Treat its
timings as provisional, not representative; rerun after the suite finishes.
Counts begin before navigation and end after the load event and timing read.
Completed bytes are CDP encoded transfer bytes, not asset gzip sizes. Unfinished
requests have unavailable bytes. Timings are navigation-start to
DOMContentLoaded and load, in milliseconds; values are single observations.

| Route | Cache | Requests | Completed bytes | Unfinished | DOMContentLoaded ms | Load ms |
|---|---|---:|---:|---:|---:|---:|
| Home | cold | 5 | 74099 | 0 | 23.7 | 24.1 |
| Home | warm | 4 | 73587 | 0 | 27.7 | 29.3 |
| Items | cold | 8 | 113735 | 3 | 31.7 | 36.1 |
| Items | warm | 6 | 113735 | 1 | 21.2 | 22.5 |
| Clients | cold | 6 | 71919 | 0 | 18.6 | 33.3 |
| Clients | warm | 4 | 66683 | 0 | 14.8 | 17.6 |
| Files | cold | 5 | 76427 | 0 | 46.7 | 60.8 |
| Files | warm | 4 | 75915 | 0 | 31.6 | 32.7 |
| Members | cold | 5 | 76607 | 1 | 15.5 | 18.6 |
| Members | warm | 4 | 76607 | 0 | 18.1 | 20.6 |
| Invitations | cold | 5 | 73741 | 0 | 20.5 | 20.8 |
| Invitations | warm | 4 | 73229 | 0 | 19.2 | 21.1 |

All navigations returned 200. There were no redirects, transport failures or
CDP cache-served events in these intervals. Cold Home, Clients, Files, Members
and Invitations each recorded a favicon 404; warm runs recorded no HTTP
errors. This repeated unnecessary request needs investigation under the UI
parent #374; no fix is claimed. The Items and Members unfinished requests are
not classified here, and must not be assumed to be SSE. The script records no
URLs, headers, bodies, cookies, credentials or secret values.

The fixture provides empty lists except Members, but this run did not assert
their visible copy or loading feedback. Populated lists, item create/edit,
client detail, list mutations, HTMX fragments, SSE connections and events,
uploads, downloads, pending, validation, failure, permission, no-JavaScript,
mobile, Spanish and dark-theme states remain unverified. Duplicate requests
other than the repeated missing favicon and per-screen script necessity remain
unverified in this snapshot. Later observations follow.

## Follow-up measurement window

The user reported the race suite and submission hooks finished before these
runs. No other workload was measured or controlled. The navigation rerun at
2026-11-16T16:30:15.640Z used the command and environment above, application
commit `12b62ccfa69a7177d3a07c965ad4cfcbc9ae4855`, and the pending harness
change that isolates local file storage. One run per cache condition:

| Route | Cache | Requests | Completed bytes | Unfinished | DOMContentLoaded ms | Load ms |
|---|---|---:|---:|---:|---:|---:|
| Home | cold | 5 | 73587 | 1 | 12.0 | 15.8 |
| Home | warm | 4 | 73587 | 0 | 18.0 | 18.1 |
| Items | cold | 7 | 113735 | 2 | 17.2 | 23.3 |
| Items | warm | 8 | 123183 | 1 | 32.9 | 34.2 |
| Clients | cold | 5 | 67195 | 0 | 24.4 | 33.4 |
| Clients | warm | 4 | 66683 | 0 | 18.6 | 21.5 |
| Files | cold | 5 | 76427 | 0 | 18.3 | 20.1 |
| Files | warm | 4 | 75915 | 0 | 13.1 | 14.5 |
| Members | cold | 5 | 77119 | 0 | 16.8 | 17.8 |
| Members | warm | 4 | 76607 | 0 | 19.9 | 21.0 |
| Invitations | cold | 5 | 73229 | 1 | 17.0 | 17.4 |
| Invitations | warm | 4 | 73229 | 0 | 19.4 | 19.6 |

All document responses were 200. Cold Clients, Files and Members each recorded
a favicon 404; unfinished requests cannot be classified from these totals.
No redirects, cache-served events or transport failures were counted. Request
counts are interval-dependent; the additional warm Items requests need
classification before being called duplicates.

The interaction run ended at 2026-11-16T16:31:52.122Z. Same application,
browser, viewport, locale, theme, network and fresh SQLite setup; sequential
actions share that fixture and browser cache. Local file storage is temporary;
SMTP and Cloudflare email environment variables are unset for the server. The
fixture contains two synthetic Items, one Client, a 30-byte text file and one
synthetic invitation at their respective steps. The peer Item is created in a
second tab. Invitation data and the one-time link are never printed. Command:

```sh
UI_MEASURE=interactions PLAYWRIGHT_PORT=58147 ./scripts/playwright-smoke.sh
```

Each row counts requests on the observed tab from immediately before the action
until the specified DOM state or download completion. Elapsed time includes
Playwright action and assertion overhead. Bytes are completed CDP encoded
bytes, excluding the long-lived stream and any unfinished response; redirect
bytes can be incomplete. SSE peer-tab POST is outside the observed tab counts.
One run, no aggregation:

| Interaction and end state | Requests | Completed bytes | Elapsed ms | SSE messages |
|---|---:|---:|---:|---:|
| Peer Item create → visible SSE refresh | 1 | 6770 | 1350.7 | 1 |
| Item create → fragment visible | 1 | 8877 | 68.2 | 0 |
| Item edit → fragment visible | 1 | 8909 | 55.0 | 0 |
| Item delete → row absent | 1 | 6813 | 109.2 | 0 |
| Client create → detail | 5 | 61903 | 95.0 | 0 |
| Client list → detail | 4 | 61903 | 96.9 | 0 |
| Client edit → heading updated | 5 | 61927 | 81.1 | 0 |
| Client archive → unarchive form | 5 | 62027 | 74.1 | 0 |
| Upload → file listed after reload | 6 | 84772 | 85.9 | 0 |
| Download → contents verified | 1 | unavailable | 105.3 | 0 |
| Injected upload 503 → retry enabled | 1 | 63 | 78.3 | 0 |
| Invitation create → revoke available | 4 | 73922 | 83.0 | 0 |
| Invitation revoke → revoke absent | 5 | 73317 | 130.5 | 0 |

Each row had zero unfinished requests. Client create/edit/archive, upload and
invitation revoke each included one redirect; redirect byte accounting may be
incomplete. Download succeeded and its contents matched, although CDP reported
one loading failure when Chrome handed the navigation to its download manager;
encoded transfer bytes are unavailable. The injected 503 is intentional; all
other rows had zero recorded HTTP errors and transport failures.

Items reached the connected SSE status and a peer mutation produced one event
and one observed refresh request. Stream connection bytes and duration are not
available from completed-request accounting. On Clients, the list exposes an
SSE endpoint but its page loads neither `app.js` nor HTMX; waiting for connected
status timed out. The missing live refresh is actionable under Client list #415
and realtime #308. Repeated favicon 404s map to shell #410. Neither fix is
claimed. The upload's held request showed a visible status and disabled button;
after injected 503 it showed an error and enabled retry. Success reloaded the
list. No claim is made about progress milestones from this small file.

Still unverified: member role/removal/ownership mutations need another member;
Client unarchive/delete and field changes, invitation resend, filter and page
mutations, validation, authorization failures, no-JavaScript, mobile, Spanish,
dark theme, item request failure, SSE disconnect/reconnect and upload progress
need dedicated scenarios. These are not zero measurements. #447 remains
partial until those scenarios and per-screen script/duplicate classification
are assessed. No performance budget is established.

## Prepared follow-up scenarios

`UI_MEASURE=gaps PLAYWRIGHT_PORT=58147 ./scripts/playwright-smoke.sh` prepares
an isolated run for Item, Client, File and Member filtering, required-field
validation, Client unarchive/delete, invitation resend, Item creation without
JavaScript, and a mobile Spanish dark-theme Item navigation. It uses the same
temporary database, local storage and disabled email delivery as the interaction
run. The mobile case sets the Spanish locale cookie before clearing the HTTP
cache. The script prints only aggregate network metrics, locale, theme canvas,
and boolean state checks; it does not print response bodies or invitation links.
It has been syntax-checked but has not yet run: selectors, state assertions,
conditions and results must be verified in a clean measurement window.

Member role, removal and transfer measurements are feasible after adding a
second synthetic user and membership. SSE reconnection, Item failure, upload
progress, Client field changes, pagination, permission failures and remaining
locale and viewport combinations also need runnable scenarios; they are not
inherently unavailable. The existing download byte count is unavailable only
from the current CDP completed-request observer because Chrome hands the
response to its download manager. Stream bytes and redirect bytes likewise need
different instrumentation; do not interpret the existing missing values as zero.

## Isolated reruns after #456 fix

On 2026-11-16 the user confirmed the #411 suite had stopped and provided an
exclusive measurement window. Runs used application commit
`88a8d9b42911299c1540759957fe1b4ffd488c31`, which includes upstream isolation
commit `5042fe34fbeb3015b641015e2fc15782f911fd91`. Each command creates a new
temporary SQLite database and local storage; bootstrap and server run with an
empty inherited environment. Darwin 27.2.0 arm64, Chrome 157.0.8103.78,
Node 26.10.0, Playwright 1.64.1, unthrottled loopback, reduced motion, blocked
service workers and one run per scenario. The navigation and interaction
conditions and measurement boundaries are as above. Commands, run sequentially:

```sh
UI_MEASURE=gaps PLAYWRIGHT_PORT=58147 ./scripts/playwright-smoke.sh
UI_MEASURE=workspace PLAYWRIGHT_PORT=58147 ./scripts/playwright-smoke.sh
UI_MEASURE=interactions PLAYWRIGHT_PORT=58147 ./scripts/playwright-smoke.sh
```

Navigation run ended 2026-11-16T17:04:51.113Z. Values are requests / completed
encoded bytes / unfinished / DOMContentLoaded ms / load ms:

| Route | Cold | Warm |
|---|---|---|
| Home | 5 / 74099 / 0 / 13.5 / 17.3 | 4 / 73587 / 0 / 15.2 / 16.7 |
| Items | 8 / 114247 / 2 / 16.6 / 18.0 | 7 / 113735 / 2 / 17.7 / 19.0 |
| Clients | 5 / 67195 / 0 / 18.3 / 25.3 | 4 / 66683 / 0 / 18.0 / 19.7 |
| Files | 5 / 75915 / 1 / 17.5 / 18.5 | 4 / 75915 / 0 / 13.2 / 14.4 |
| Members | 5 / 76607 / 1 / 13.7 / 14.8 | 4 / 76607 / 0 / 15.3 / 16.3 |
| Invitations | 5 / 73229 / 1 / 14.8 / 15.2 | 4 / 73229 / 0 / 16.7 / 17.0 |

Cold Home, Items and Clients each recorded a favicon 404. All document
responses were 200, with no recorded redirects or transport failures.
Unfinished requests have unavailable bytes and are not individually classified.

Interaction run ended 2026-11-16T17:05:08.071Z. Values are requests / completed
encoded bytes / elapsed ms, using the interaction boundaries above:

| Interaction | Result |
|---|---|
| Peer Item SSE event and refresh | 1 / 6770 / 1383.5; one SSE event |
| Item create / edit / delete | 1 / 8877 / 66.9; 1 / 8909 / 68.4; 1 / 6813 / 89.0 |
| Client create / detail / edit / archive | 5 / 61903 / 97.3; 4 / 61903 / 80.0; 5 / 61927 / 83.0; 5 / 62027 / 67.6 |
| File upload / download / injected 503 | 6 / 84772 / 139.3; 1 / unavailable / 381.1; 1 / 63 / 53.7 |
| Invitation create / revoke | 4 / 73922 / 84.2; 5 / 73317 / 74.8 |

All rows had zero unfinished requests. The download contents matched; CDP
reported a loading failure on download handoff, so its encoded bytes remain
unavailable by this observer. Upload failure was an injected 503; pending status
was visible and retry became enabled. Other rows had no HTTP or transport
errors. Client mutations, upload and invitation revoke each counted a redirect;
redirect bytes may be incomplete. Client SSE remains disconnected as described
above. The peer POST is excluded from the observed tab's request count.

Gap run ended 2026-11-16T17:04:37.197Z. It uses a fresh fixture, warm cache
for sequential desktop scenarios, and the separate no-JavaScript and mobile
contexts in the script. Filters are direct GET navigations ending at `load`;
all four showed empty results. Values are requests / completed encoded bytes /
unfinished / elapsed ms:

| Scenario | Result |
|---|---|
| Item filter | 7 / 119209 / 1 / 64.1 |
| Client filter | 4 / 67342 / 0 / 56.3 |
| File filter | 4 / 76091 / 0 / 106.5 |
| Member filter | 4 / 76683 / 0 / 125.6 |
| Client required validation | 0 / 0 / 0 / 76.4; browser reported invalid |
| Client unarchive / delete | 5 / 61900 / 0 / 92.3; 5 / 66683 / 0 / 140.1 |
| Invitation resend | 4 / 73922 / 0 / 85.9 |
| Item create without JavaScript | 3 / 39754 / 0 / 84.4 |
| Mobile Spanish dark Items navigation | 8 / 123737 / 1 / 60.4 |

Mobile used 390×844, scale 1, Spanish locale cookie, dark system color scheme,
and a cleared HTTP cache after locale setup. The observed HTML language was
`es` and canvas color `#111826`; it recorded a favicon 404. Other gap rows
recorded no HTTP or transport errors. Client unarchive/delete and no-JavaScript
create each counted one redirect. Elapsed values include automation overhead;
they are observations, not budgets. Member role/removal/transfer, pagination,
Client fields, SSE reconnect, Item failure, upload progress and permission
states remain unmeasured, as does per-request duplicate classification. #447
remains partial.
