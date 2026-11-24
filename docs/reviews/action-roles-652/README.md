# Issue 652: request-form UX verification

**Partial review; completion blocked by gaps below.** Target `ab21d98ffa9b4389f33bc04a2ffa5d40b2787a16`; comparison parent `4c58ed48f3318aa378dfd43362fcd3044c8f42ea`. Real local servers, disposable SQLite database, synthetic workspace and owner/member accounts, Playwright. Screenshots visually inspected; private link input hidden before capture, with no credentials or link tokens visible. Hiding the input changes the captured rendering and does not test its readability.

## Rendered comparison

Desktop captures: 1440×900, light, owner, empty Clients, active form with open engagement stage; parent database copied from target fixture.

- Clients: [before](before-clients.png) / [after](after-clients.png). Share trigger and Open/Configure became outlined controls; Copy became primary. Native summary, button and anchors retained. Menu remains legible.
- Settings: [before](before-settings.png) / [after](after-settings.png). Copy and Save became primary; Open and delivery navigation outlined; Rotate/Disable outlined in danger color. Actions remain distinguishable.
- Target, 390×844 dark: [Clients](mobile-clients.png), [Settings](mobile-settings.png). Both document widths were 390px; no horizontal overflow in these states.

## Actions and outcomes

1. Owner opened disabled Clients share menu, navigated to settings, created an open stage and enabled the request form with synthetic consent. Active controls rendered in Clients and Settings.
2. On Clients, focused Share, pressed Enter, then Tab: focus reached link input, Copy, Open, Configure. Focus outline computed `solid` on each inspected subsequent control. Enter on Copy showed “Link copied.” Clipboard contents were not inspected.
3. Member could open Clients (HTTP 200), saw no Share action, and direct request-form settings navigation returned HTTP 403.
4. With JavaScript disabled in a separate browser context, owner signed in, saved changed consent through the ordinary form, and opened native Clients share disclosure containing the Open link. This does not test destructive confirmations without JavaScript.

## Commands

- `git archive ab21d98^` into a temporary directory; build its `./cmd/server` and build target `./cmd/server` using `go -C <directory> build`.
- Bootstrap owner with `go -C <target> run ./cmd/bootstrap-admin`; seed member with `go -C <target> run ./internal/browsertest/seedmember`; start both servers on loopback with SQLite and disposable encryption configuration. Copy fixture using SQLite `.backup` before starting comparison server. Servers stopped after review.
- `go -C <target> test ./internal/web -run 'Intake|ClientIntake' -count=1` passed.

## Gaps

No populated-client, long-content, unavailable-link error, public submission, rotate/disable confirmation, full keyboard traversal, automated accessibility or asset drift checks in this target run. No app-wide navigation inventory or rendered parent mobile/dark review. Screenshots alone do not establish accessibility conformance. Original report described a run executed from a different worktree; its results are excluded here.
