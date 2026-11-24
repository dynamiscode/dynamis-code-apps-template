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

## Additional browser review

Target server restarted with the same disposable workspace. Playwright, owner, 390×844 dark unless noted. New captures inspected for secrets; each request-link input was replaced in the DOM with a nonfunctional `example.invalid` URL before capture. This is a rendering-only substitution, not a test of the real URL or clipboard.

- [Populated Clients](populated-mobile.png): added one synthetic client with a 65-character name, opened Share. List and menu visible; document width 390px. The synthetic URL is readable at its beginning but clipped by the single-line field at this width; remaining text requires horizontal field scrolling.
- [Long consent settings](long-consent-mobile.png): saved 1,423 characters of synthetic consent. Consent preview wraps and settings document width remains 390px.
- **Finding: public form horizontal overflow with long consent.** In a 390×844 public-form viewport, the same consent label expands document scroll width to 10,387px. The visitor must scroll horizontally to read the consent and may miss content. The label/span caused the overflow. This was observed in the target revision, not assessed against the parent; no application code changed.
- [Public form error](public-error-mobile.png): entered a synthetic name and accepted consent, then submitted without email or phone. Page displayed “Enter a phone number or email address to continue.” Screenshot shows viewport only; long consent overflow remains. No actual link token is shown.
- Rotate and Disable each displayed a confirmation warning that existing links and QR codes stop working. Canceling Rotate left the form active. In a separate page, accepting Rotate left it active; accepting Disable changed status to disabled. No old-link invalidation test performed.

## Commands

- `git archive ab21d98^` into a temporary directory; build its `./cmd/server` and build target `./cmd/server` using `go -C <directory> build`.
- Bootstrap owner with `go -C <target> run ./cmd/bootstrap-admin`; seed member with `go -C <target> run ./internal/browsertest/seedmember`; start both servers on loopback with SQLite and disposable encryption configuration. Copy fixture using SQLite `.backup` before starting comparison server. Servers stopped after review.
- `go -C <target> test ./internal/web -run 'Intake|ClientIntake' -count=1` passed.

## Gaps

Public long-consent overflow remains unresolved. No unavailable-link error, successful public submission, old-link invalidation, full keyboard traversal, automated accessibility or asset drift checks in this target run. No app-wide navigation inventory or rendered parent mobile/dark review. Screenshots alone do not establish accessibility conformance. Original report described a run executed from a different worktree; its results are excluded here.
