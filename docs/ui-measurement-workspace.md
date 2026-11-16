# Workspace navigation measurements

Partial evidence for #447, using the [common method](ui-measurement.md). This
does not complete #447 or set a budget. The measurement script was added on
top of application commit `d438f9d39a7063c0292ce4731c81977ca8af9a81`.

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
unverified. These are prerequisites for completing #447, not zero values.
