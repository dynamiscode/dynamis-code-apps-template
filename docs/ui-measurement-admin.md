# Administration and data measurement preparation

Issue #448 covers general and email settings, API tokens, SCIM provisioning,
webhooks and deliveries, audit history, import preview, and export. The common
[measurement method](ui-measurement.md) applies. The single run below is a
partial baseline; no budgets or performance fixes are established.

The separate `sh scripts/ui-measurement-admin.sh` command creates a temporary
SQLite fixture and runs `scripts/ui-measurement-admin.mjs`. Install dependencies
with `npm ci` and use the Chrome prerequisites in the common method first.
The current harness builds the binaries, then starts bootstrap and server with
an allowlisted environment through `scripts/ui-measurement-admin-env.sh`: a
temporary SQLite database and local file store, a test-only webhook key,
loopback HTTP, and `OTEL_SDK_DISABLED=true`. Caller storage, telemetry and mail
provider settings are excluded. `node --test scripts/ui-measurement-admin-env.test.mjs`
checks this boundary without starting the server or collecting measurements.
It records cold and warm navigation to eight settings pages, with the same CDP
network definitions as #446. Each cold navigation clears the HTTP cache after
sign-in; warm repeats the same page. A single context is reused. Each interval
ends after the load event and visible main heading. The audit lazy request may
overlap that interval and must not be interpreted as a separate fragment result.
The script creates one synthetic webhook with a fixed test-only encryption key
after the initial navigation runs. It does not trigger a delivery. Its secret
appears once in the browser but is never recorded. Deliveries navigation is
measured cold and warm with an empty delivery list. The audit fragment is a
separate warm `fetch` with the same header as `forms.js`, from an audit page
loaded with `?all=1`; it is not a measurement of the automatic fetch or DOM
swap. The harness also prepares an observation of the actual automatic audit
request and DOM replacement, with a best-effort loading-indicator observation;
that added instrumentation has not been executed. The import preview submits
one synthetic CSV row without confirming it.
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
counter alone as a failed download. Loading feedback, SSE and per-request script
attribution are not captured.

The output contains aggregate counters, safe HTTP error classifications and
environment metadata, never URLs, response bodies, credentials or headers.
Errors from the browser script are suppressed because they can contain page
contents or URLs. A nonzero exit therefore requires local investigation; do not
share unsanitized diagnostics.
Run only against the isolated synthetic fixture. Save the exact command, commit,
output and fixture conditions when results are collected.

| Screen or interaction | Prepared coverage | Unverified |
|---|---|---|
| General and email settings | Cold/warm navigation | Save, validation, failure and provider states |
| Tokens and SCIM | Cold/warm navigation | Creation, revocation and one-time secret states |
| Webhooks | Cold/warm navigation; empty deliveries navigation after synthetic creation | Populated, error and pending delivery states |
| Audit | Cold/warm navigation; separate equivalent synthetic warm fragment fetch | Newly prepared automatic fetch and DOM swap instrumentation, loading/failure and no-JavaScript fallback |
| Import | Cold/warm navigation; one-row CSV preview | Validation, failure and confirmation |
| Export | Cold/warm navigation; download and local decoded file size | Download transfer bytes and failure |

Every row also lacks permission, mobile, Spanish, dark theme, keyboard and
no-JavaScript verification. The fixture has no deliveries; populated and pending
delivery fixtures remain necessary.
Do not capture one-time token or webhook secrets, SMTP credentials, CSRF fields,
export bodies, endpoint URLs, traces or screenshots. Measurements and additional
state coverage are still needed before identifying duplicate requests, proposing
measured fixes, or completing #448.

## Single isolated run

On 2026-11-16T16:32:36.475Z, commit
`ef822879b8ffb51d6ab91d9f533c830a07f665f7`, Darwin 27.2.0 arm64,
Go 1.27.4, Node 26.10.0, Chrome 157.0.8103.78 and Playwright 1.64.1:

```sh
npm ci
sh scripts/ui-measurement-admin.sh > /tmp/ui-448-measurement.json
```

**Isolation caveat:** This run used the earlier harness, which inherited caller
application configuration and did not set a temporary local file store. The
session had `OTEL_EXPORTER_OTLP_ENDPOINT`, `STORAGE_DRIVER`, `STORAGE_LOCAL_PATH`,
`SMTP_HOST` and `CLOUDFLARE_EMAIL_ACCOUNT_ID` unset, but other inherited settings
were not fully inventoried. The default local store was not isolated. External
export or provider inactivity is not verified; do not treat this run as a fully
isolated baseline. The revised environment boundary has not been exercised by a
measurement run.

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
| General | cold | 4 | 59055 | 18.0 | 23.2 |
| General | warm | 3 | 58543 | 14.9 | 17.3 |
| Email | cold | 5 | 76653 | 24.8 | 26.1 |
| Email | warm | 4 | 76141 | 13.7 | 14.7 |
| Tokens | cold | 6 | 73975 | 56.7 | 59.6 |
| Tokens | warm | 5 | 73463 | 16.8 | 19.2 |
| SCIM provisioning | cold | 4 | 58875 | 19.2 | 19.4 |
| SCIM provisioning | warm | 3 | 58363 | 13.8 | 15.0 |
| Webhooks | cold | 5 | 74321 | 28.5 | 28.7 |
| Webhooks | warm | 4 | 73809 | 16.0 | 16.2 |
| Audit | cold | 5 | 59775 | 19.5 | 20.4 |
| Audit | warm | 4 | 59263 | 14.6 | 14.7 |
| Import | cold | 5 | 59439 | 19.1 | 19.1 |
| Import | warm | 4 | 58927 | 16.5 | 16.6 |
| Export | cold | 5 | 72744 | 16.4 | 16.4 |
| Export | warm | 4 | 72232 | 14.7 | 14.7 |
| Webhook deliveries | cold | 5 | 72710 | 13.9 | 13.9 |
| Webhook deliveries | warm | 4 | 72198 | 12.8 | 12.9 |

Every cold navigation recorded one favicon HTTP 404; warm navigations recorded
no HTTP errors. All navigations had zero redirects, transport failures and
unfinished requests. The token page reported one cache event in each run; the
other pages reported none. Per-resource request attribution was not collected,
so duplicate and unnecessary requests cannot be assessed beyond the repeated
missing favicon. Investigate the favicon in the owning shared-shell work; no fix
is claimed here.

| Interaction | Requests | Completed encoded bytes | Elapsed ms | Other |
|---|---:|---:|---:|---|
| Audit fragment fetch | 1 | 1994 | 7.3 | Warm; separate fetch, not automatic swap |
| CSV import preview | 4 | 60436 | 82.9 | Warm; one synthetic row; no confirmation |
| Export download | 1 | unavailable | 118.0 | Warm; 2056 decoded file bytes; wire bytes unavailable |

The interactions had no redirects, HTTP errors, cache events or unfinished
requests. Chrome reported one transport failure for the download, while
Playwright returned a completed file; its cause is unverified. The other two
interactions reported no transport failures. The download observer reported
zero completed encoded bytes; that is not a measurement of its wire size.
`npm ci` warned that the installed Node version is below selenium-webdriver's
declared minimum; this harness uses Playwright, and the run completed. No
browser performance measurement was repeated to estimate variability.
