# HTMX review: Items, sharing and Clients (#435)

Review baseline: `81c496187ec060f359a38e844859c23c5e265931`, 2026-11-16.
This is a decision inventory, not an implementation or evidence that the
proposed behavior has shipped. Route migrations own the follow-up. The existing
general HTMX upgrade evidence is in the template's capability ledger.

## Shared rules for the implementation tickets

Keep ordinary GET links and POST forms, session CSRF and server-side workspace
authorization. Mutation successes do not push history; query GETs and pagination
retain shareable URLs and normal browser history. Keep the query on redirects
and realtime refresh. Never preload or cache a one-time share URL. Preserve
`private, no-store`, referrer and robots protections on public shares, and
`no-store` on secret-bearing responses. Do not send security fields to WebMCP.
Maintain labeled controls, focus and error announcements, keyboard operation,
responsive table/cards, English and Spanish, and light/dark/system themes.
Pending controls must prevent duplicates and failures must restore controls.

The following rows use **retain** for an existing choice and **defer** for an
unadopted enhancement. A deferred action has no HTMX trigger or swap target:
its ordinary form/link remains the trigger and full-page navigation is its
target. No new timeout or automatic retry is approved. For navigation, the
browser handles history; for form POST success, the server's 303 redirect
returns to the stated page unless otherwise noted. Shared service authorization
and CSRF apply even where controls are hidden. Permission and unexpected errors
remain full-page errors; do not treat an error response as a success.

| Screen / action | Current behavior and decision | Status, state, synchronization and fallback | Owner |
|---|---|---|---|
| Items create | **Retain** submit POST to collection; `hx-target="#item-list"`, `outerHTML`, `hx-sync="this:drop"`. Form has idempotency key and persisted non-secret draft. | Enhanced success 200 fragment; ordinary success 303. Invalid 422, limit/idempotency 409 on ordinary form. `renderItems` converts its enhanced errors to 200 with `HX-Trigger: form-error`; preserve draft and visible error. Busy state prevents duplicate submit. Empty/results are in list fragment. | #413 |
| Items edit/status/delete | **Retain** submit POST to item and status change autosave, same target/swap and per-form `this:drop`. Delete uses `action=delete`, confirmation and optimistic hide. Version enforces conflict detection. | 200 fragment / ordinary 303; invalid 400/422, stale version 412, forbidden 403, missing 404 and server failure 500; rendered item errors become enhanced 200 as above. Restore optimistic row on failure; focus alert after swap. Do not overwrite a different dirty row when replacing the list: verify and address this in #413. Ordinary submit remains usable without script. | #413 |
| Items search/status/sort/limit, pagination and columns | **Defer** HTMX navigation. GET query form, clear and next links navigate; column preference POST redirects. | GET 200 or invalid query 400; preference success 303, failures 400/403/500. Preserve query and cursor rules, history, empty and no-visible-column states. No HTMX request synchronization. | #413 |
| Item share create | **Retain** explicit POST submit, `#item-list` outerHTML; no `hx-sync` currently. One-time link is shown in response. | 200 fragment or full page; invalid lifetime/create 422, invalid action 400, missing 404; rendered enhanced errors become 200. Do not snapshot the secret into HTMX history, preload, logs or storage. Verify overlapping submits and dirty rows before keeping list-wide swap. Ordinary POST works. | #414 |
| Item share revoke | **Defer** targeted revoke: ordinary POST with confirmation and 303 back to list; conflict 409, invalid action 400. | No swap or `hx-sync`; busy handling is script enhancement, ordinary form remains. Keep explicit consequence and permission gating. No secret needs re-display. | #414 |
| Public share | **Defer** HTMX: ordinary read-only GET; valid projection contains only title/status, unavailable response covers invalid, expired and revoked links. | No mutation, sync, history snapshot or preload. Preserve response privacy headers and rate limit. | #414 |
| Clients list/query/columns | **Defer** HTMX navigation. Ordinary GET filters/sort/page and preference POST retain URLs; list can return `client-list` for `HX-Request`. | GET 200, invalid query/cursor 400; preferences redirect on success. Empty, permissions and failures stay visible. No HTMX sync for ordinary forms. | #415 |
| Clients live refresh | **Adopt the intended existing fragment refresh**, subject to fixing the missing script wiring in #415. The template has `#client-list[data-events]` and server returns `client-list`, but `clients.html` loads neither `htmx.min.js` nor `app.js`; the EventSource refresh therefore does not run on this baseline. | On `client.changed` or `resync`, GET current path and query, replace `#client-list` outerHTML with 200 fragment, no history push. Defer while focused, dirty or submitting; show waiting/reconnect state. Handle failed fetch without discarding input. Without JS, reload/navigation remains available. Both stream and fragment require `identity.ResourcesRead`; #415 must preserve it. | #415 |
| Client create | **Defer** HTMX. POST collection redirects to detail; validation renders list and draft. | Success 303; invalid 422, permission/error via shared handler. Busy indicator with JS; native POST without it. Protect unsent create/filter inputs from live refresh before enabling it. | #415 |
| Client detail edit and values set/clear | **Defer** HTMX. POST to detail or `/values/{fieldId}` and `/clear`; redirect to detail. | 303 success, 412 stale version, 409 archived field, 422 invalid input/option, permission and server errors via shared handler. Preserve drafts where available; value conflict currently renders stored values, so verify recovery and prevent silent loss in #416. No automatic retries. | #416 |
| Client archive/restore/delete | **Defer** HTMX. POST `/archive`, `/unarchive`, `/delete`; delete redirects to list, others to detail. | 303 success; shared permission/error handling. Delete is permanent and gated to owner/admin; retain consequences and ordinary submission without JS. No swap or sync. | #416 |
| Client field create/archive/restore/delete | **Defer** HTMX. POST collection or `/{fieldId}/archive`, `/restore`, `/delete`; redirect to fields. | 303 success; create invalid 422 or limit 409; deleting active field 409. Owner/admin management and CSRF enforced server-side. Preserve field draft on create errors and explain permanent deletion. | #416 |
| Field option add/delete | **Defer** HTMX. POST `/{fieldId}/options` or `/options/{optionId}/delete`; redirect to fields. | 303 success; add invalid/non-choice 422, limit/archived 409; shared permission/error handling. No swap or sync; preserve ordinary forms and confirmation. | #416 |

## Core and extension decisions

* **Response status:** retain existing `noSwap=[204,304,"4xx","5xx"]` in
  `app.js` and the item renderer's explicit 200/form-error convention until
  #413 and #414 implement and test an alternative. Do not globally swap error
  pages. HTMX 4 `hx-status` can target specific statuses, but adopting it
  requires a bounded error fragment, conflict/draft recovery and browser tests.
* **Synchronization:** retain `this:drop` on each item create/edit form, not
  a claim of cross-form serialization. Share creation and list refresh can
  overlap mutations; #413–#415 must test concurrent responses and protect
  other dirty forms before replacing a whole section. A dropped or aborted
  request does not prove a server mutation was cancelled.
* **Inheritance and DELETE:** keep attributes explicit on each enhanced form.
  HTMX 4 inheritance is explicit; do not assume parent attributes apply. Keep
  POST action buttons and their hidden CSRF/version inputs; do not migrate to
  `hx-delete` without testing input encoding and ordinary form fallback.
* **History and timeouts:** no `hx-boost`, history snapshots or mutation URL
  pushes are adopted. Preserve native query navigation. Defer request timeouts
  until a measured need and safe ambiguous-mutation recovery exist.
* **Morphing:** defer core morph and morph extensions. Preserving a DOM node
  alone does not establish correct dirty values, version, focus or error state.
  Test those guarantees before changing the current outerHTML swap.
* **Preload:** defer `hx-preload`; no measured benefit, and speculative requests
  and cached private or one-time content are undesirable.
* **SSE:** retain the existing EventSource protocol and deferred refresh design;
  defer `hx-sse` extension until it demonstrably preserves resync, access
  control, reconnect status, query state and dirty-input protection. Items
  currently load both required scripts; Clients do not.
* **hx-live:** defer. Current server-rendered forms, validation and realtime
  refresh already cover the intended interaction; no concrete benefit justifies
  another state mechanism or its compatibility/security testing.

## Evidence and remaining verification

Source: [item template](../internal/web/templates/item-list.html),
[item handler](../internal/web/handler.go), [sharing handler](../internal/web/sharing.go),
[client list template](https://github.com/dynamiscode/dynamis-code-apps-template/blob/81c496187ec060f359a38e844859c23c5e265931/internal/web/templates/clients.html),
[client handler](https://github.com/dynamiscode/dynamis-code-apps-template/blob/81c496187ec060f359a38e844859c23c5e265931/internal/web/clients.go), [client lifecycle](https://github.com/dynamiscode/dynamis-code-apps-template/blob/81c496187ec060f359a38e844859c23c5e265931/internal/web/client_lifecycle.go),
[field lifecycle](https://github.com/dynamiscode/dynamis-code-apps-template/blob/81c496187ec060f359a38e844859c23c5e265931/internal/web/client_fields.go), [field options](https://github.com/dynamiscode/dynamis-code-apps-template/blob/81c496187ec060f359a38e844859c23c5e265931/internal/web/client_options.go),
[realtime client](../internal/web/assets/app.js), [form handling](../internal/web/assets/forms.js),
and [client stream](https://github.com/dynamiscode/dynamis-code-apps-template/blob/81c496187ec060f359a38e844859c23c5e265931/internal/web/clients_events.go). Existing handler and
browser tests are evidence of their assertions, not proof of every matrix cell.

Upstream: [HTMX 4.0.1 release](https://github.com/bigskysoftware/htmx/releases/tag/v4.0.1)
(2026-11-01), [4.0.1 migration guidance](https://github.com/bigskysoftware/htmx/blob/v4.0.1/www/src/content/docs.md),
[4.0.1 extensions](https://github.com/bigskysoftware/htmx/tree/v4.0.1/www/src/content/extensions)
and [4.0.1 documentation](https://github.com/bigskysoftware/htmx/blob/v4.0.1/www/src/content/docs.md).
Context7 indexed 4.0.0 rather than 4.0.1. The upstream directory returned HTTP
200 to curl, but the review browser could not fetch its contents. Exact 4.0.1
extension compatibility remains unverified; check it before adopting anything.

Unverified in this review: each failure and permission state in both locales,
themes and viewport sizes; no-JavaScript journeys; keyboard/focus and screen
reader behavior; simultaneous item forms and share responses; actual client
realtime wiring after migration; and whether HTMX history can capture a share
secret under future navigation changes. #413–#416 must add automated journey
and accessibility evidence, including 4xx non-swap, reconnect, unsaved input,
conflict and request failure. Automated accessibility checks do not prove WCAG
conformance. No manual review is implied by this document.
