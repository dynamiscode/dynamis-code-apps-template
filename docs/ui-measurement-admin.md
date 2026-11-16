# Administration and data measurement preparation

Issue #448 covers general and email settings, API tokens, SCIM provisioning,
webhooks and deliveries, audit history, import preview, and export. The common
[measurement method](ui-measurement.md) applies. No measurements have been run
for this slice, and no budgets or performance fixes are established.

The separate `sh scripts/ui-measurement-admin.sh` command creates a temporary
SQLite fixture and runs `scripts/ui-measurement-admin.mjs`. Install dependencies
with `npm ci` and use the Chrome prerequisites in the common method first.
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
swap. The import preview submits one synthetic CSV row without confirming it.
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
| General and email settings | Cold/warm navigation | All results, save/validation/failure and provider states |
| Tokens and SCIM | Cold/warm navigation | All results, creation/revocation and one-time secret states |
| Webhooks | Cold/warm navigation; empty deliveries navigation after synthetic creation | All results, populated/error/pending delivery states |
| Audit | Cold/warm navigation; separate equivalent fragment fetch | All results, actual automatic fetch and DOM swap, loading/failure and no-JavaScript fallback |
| Import | Cold/warm navigation; one-row CSV preview | All results, validation, failure and confirmation |
| Export | Cold/warm navigation; download and local decoded file size | All results, download transfer bytes and failure |

Every row also lacks permission, mobile, Spanish, dark theme, keyboard and
no-JavaScript verification. The fixture has no deliveries; populated and pending
delivery fixtures remain necessary.
Do not capture one-time token or webhook secrets, SMTP credentials, CSRF fields,
export bodies, endpoint URLs, traces or screenshots. Measurements and additional
state coverage are still needed before identifying duplicate requests, proposing
measured fixes, or completing #448.
