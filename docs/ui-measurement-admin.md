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
The output contains aggregate counters, safe HTTP error classifications and
environment metadata, never URLs, response bodies, credentials or headers.
Run only against the isolated synthetic fixture. Save the exact command, commit,
output and fixture conditions when results are collected.

| Screen or interaction | Prepared coverage | Unverified |
|---|---|---|
| General and email settings | Cold/warm navigation | All results, save/validation/failure and provider states |
| Tokens and SCIM | Cold/warm navigation | All results, creation/revocation and one-time secret states |
| Webhooks | Cold/warm navigation, empty fixture | All results, populated/error/pending states and deliveries page |
| Audit | Cold/warm navigation | All results, separate lazy fragment timing/bytes, loading/failure and no-JavaScript fallback |
| Import | Cold/warm navigation | All results, preview upload, validation, failure and confirmation |
| Export | Cold/warm navigation | All results, download request and bytes, failure |

Every row also lacks permission, mobile, Spanish, dark theme, keyboard and
no-JavaScript verification. The fixture contains no webhook or deliveries;
creating a safe synthetic webhook and isolated delivery state remains necessary.
Do not capture one-time token or webhook secrets, SMTP credentials, CSRF fields,
export bodies, endpoint URLs, traces or screenshots. Separate interaction
measurements and their boundaries are still needed before identifying duplicate
requests, proposing measured fixes, or completing #448.
