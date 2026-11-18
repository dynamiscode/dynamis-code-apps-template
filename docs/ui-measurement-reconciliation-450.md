# UI measurement reconciliation and budget recommendation

Issue #450 reconciles evidence prepared for #439. The available screen timings
are single, unthrottled observations. They support a baseline record, not
statistical targets or user-facing performance guarantees. No performance fix
is claimed as implemented here.

## Comparable evidence

The asset inventory in [the measurement method](ui-measurement.md) was captured
at `81c496187ec060f359a38e844859c23c5e265931` on 2026-11-16. It lists embedded
CSS and JavaScript raw and deterministic gzip bytes, tool versions, and the
reproduction command. Fonts are system fonts; no font files are embedded.
Those gzip values compare files and do not establish HTTP transfer size.

The isolated admin run in [admin measurement evidence](ui-measurement-admin.md)
used commit `763a333db4afb6c1f6dda84acfb33e42d2a6854e`, Chrome 157.0.8103.78,
Playwright 1.64.1, Node 26.10.0, Go 1.27.4, a fresh SQLite fixture, 1280×900
viewport, and one unthrottled run. Cold and warm navigation covered eight
settings pages; a separate deliveries navigation had an empty list. The run
also measured audit fetch, one-row import preview, and export. Fixture details
are recorded with the result. Concurrent checks may have affected timings, so
they are not claimed as uncontended.

Across the eight page cold runs, requests ranged from 4 to 6, completed encoded
bytes from 58,875 to 76,653, DOMContentLoaded from 12.8 to 23.0 ms, and load
from 13.8 to 26.8 ms. Warm runs ranged from 3 to 5 requests, 58,363 to 76,141
completed bytes, 11.4 to 16.5 ms DOMContentLoaded, and 12.5 to 17.5 ms load.
Each cold run reported one favicon 404; warm runs reported none. Eight pages
each requested one document, `app.css`, and `forms.js`; these counts do not show
whether requests outside the interval exist or whether an asset is unnecessary.

The synthetic audit fetch recorded one request, 1,994 completed encoded bytes,
and 5.0 ms elapsed. The import preview recorded four requests, 60,436 completed
encoded bytes, and 85.3 ms elapsed. Export recorded one request and 118.1 ms
elapsed; its wire bytes are unavailable, while its temporary decoded file was
2,057 bytes. The automatic audit replacement appeared at 45.3 ms after
navigation start, with the loading indicator observed; loading duration and
feedback accessibility were not measured. One Chrome transport-failure count
for export coexisted with a completed file, so the transport classification
remains unresolved.

The separate state run recorded audit HTTP 503 feedback, invalid CSV feedback,
audit no-JavaScript table fallback, and webhook pending, failed, and delivered
states. These are state checks, not timing measurements. See the admin evidence
for permission and localization limits.

## Budget decisions

Do not set numeric page-load, request-count, or transfer-byte budgets from these
data. Each page and interaction has one run, the fixture is sparse, network is
unthrottled, and no variability or representative populated-state measurements
exist. The favicon 404 appeared on each cold navigation in one run; repeat
confirmation is needed before it becomes a reliability threshold.

Use the following as collection gates, not performance budgets: retain cold and
warm labels; report page navigation separately from HTMX, SSE, upload, and
download; report unavailable bytes as unavailable; and collect repeated runs
with fixed fixture and environment before proposing thresholds. The
measurement method provides the commands and result schema.

## Coverage and owning work

The evidence measures eight settings-page navigations and one empty-list
deliveries navigation, plus audit fetch, import preview, and export noted
above. It does not establish comparable measurements for workspace home, Items
list/create/edit, Clients list/detail, Files list/upload, members/invitations,
deliveries beyond navigation, account security/sessions/notifications,
setup/login/MFA/recovery, public share, or consent. Populated data, upload wire
bytes, SSE behavior, per-request script initiators, broader loading states, and
cross-locale/theme performance remain unverified.

The cold favicon 404 is the only observed candidate for investigation by the
shared web-shell owner in issue #425. One run does not establish repeatability,
need for a fix, or cause. Current request attribution supports no other specific
performance fix. Do not assign optimization work from aggregate counts alone.
The remaining screen runs and attribution belong with their route tickets in
#439; add owning ticket links when measurements identify a concrete fix.

Reproduce asset inventory with the command in [the measurement method](ui-measurement.md).
Reproduce admin measurements with `sh scripts/ui-measurement-admin.sh` after
installing dependencies. Never publish credentials, headers, response bodies,
secret URLs, or raw traces.
