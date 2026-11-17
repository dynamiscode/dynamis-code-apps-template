# Notification center migration evidence (#429)

## Routes and behavior

- `GET /notifications` renders the recipient's latest 100 records and unread
  count. An empty result includes links to reload and manage preferences.
- `POST /notifications/{notificationId}` keeps ordinary CSRF-protected form
  submission and redirects to the list after the shared identity service marks
  that recipient's record read.
- `GET /notifications/events` streams only the authenticated recipient's
  records. The page embeds the newest rendered row as the initial `after`
  cursor, or an empty cursor for an empty inbox. `Last-Event-ID` takes
  precedence and resumes after the last delivered record. An invalid or expired
  cursor emits `resync`; the browser reloads the full list.
- The stream emits a `close` event at its configured lifetime and ends the
  response. The browser leaves native `EventSource` reconnection enabled, so it
  reconnects with its last event ID or, before any event arrives, the page
  cursor. This replays notifications created during the connection gap.
- Targeted mark-read remains deferred under the #438 HTMX review. The full-page
  POST/303 path avoids a second mutation response, focus, concurrent-count and
  failure contract while preserving ordinary form behavior.

The page uses generated templ, shared account navigation components, DaisyUI
list and badge classes, localized copy, and the existing theme tokens. Native
links and forms remain usable without JavaScript.

## Security boundary

The page requires a session. Mark-read requires that session and CSRF token; the
identity service receives the current user ID. SSE uses the authenticated user
ID for cursor lookup and event queries. Event payloads contain notification ID,
type, title, body and UTC creation time only. The response remains no-store.

## Verification

- Focused Go notification tests: pass, 3 tests. They cover rendered cursor,
  badge markup, CSRF rejection, recipient-scoped events, lifetime close, replay
  from `Last-Event-ID`, and an empty-inbox gap before any event ID exists.
- Focused HTTP middleware tests: pass, 3 tests; notification SSE bypasses the
  generic request deadline and concurrency limit.
- `make test`, `make lint`, `make templ-check`, `make css-check`, and
  `git diff --check`: pass. `make ci-local` also passes on the middleware fix,
  including `go test -race ./...`, dependency/generation checks, build, and
  secret scan.
- `make accessibility-smoke`: pass in English and Spanish; notifications has
  zero axe violations in both locales.
- Standard notification Playwright suite: 6 passed, 2 opt-in tests skipped;
  covers four desktop/mobile light/dark screenshots, keyboard navigation,
  Spanish, and no-JavaScript empty state.
- Opt-in no-JavaScript mark-read browser test: pass. It uses
  `PLAYWRIGHT_SEED_NOTIFICATION=1` with the smoke script's disposable,
  environment-cleared SQLite database.
- Real-browser lifetime reconnect: pass on port 58129 with a one-minute stream.
  The test inserts a row after the browser receives lifetime `close`, then
  checks native replay.
- CI on the updated PR head and integration validation if #424 reaches main
  first remain pending.

## Measured 504

The first real-lifetime browser run returned HTTP 504 after 30.029 seconds on
`GET /notifications/events?after=`, matching the default
`HTTP_REQUEST_TIMEOUT`. The shared middleware exempted workspace item/client
streams but not this route. `/notifications/events` now uses the existing
streaming path exemptions for request deadline and concurrency only. Request
rate limits and the notification handler's per-user stream caps remain active.
The middleware regression and one-minute browser replay pass with this fix.

`docs/web.md` now records the narrow route inventory and realtime cursor
contract. Shared docs still need reconciliation at integration.

## Shared Tailwind overlap with #424

At validation, `origin/main` is `11cf5bc5aa959e056b63a145c30f921ab2127f98`.
The live #424 worktree adds `@source "./webhooks.templ";`. This change adds
`@source "./notifications.templ";` and enables DaisyUI `list` and `badge`.
Preserve both sources and both plugin includes when either change reaches main
second; regenerate `assets/components.css` and rerun `make css-check` on the
updated main. On the requested base, the generated CSS grows by 2,235 bytes in
one minified-line replacement, adding `.list`, `.list-row`, `.badge` and
`.badge-primary` rules.

`docs/capabilities.md` separately records `TestAgentSurfacesLiveSmoke` returning
504 after 35 seconds under full race-suite load on 2026-11-10; that test passed
alone with race.
