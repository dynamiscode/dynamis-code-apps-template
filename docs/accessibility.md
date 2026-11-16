# Accessibility Evidence

Standard: WCAG 2.2 Level AA  
Review date: 2026-11-16
Owner: template maintainer

## Result

The repeatable runner is `npm run test:a11y` against a bootstrapped local
server; `make accessibility-smoke` runs it once per locale. It fails on
critical or serious axe violations, keyboard order, missing focus visibility,
horizontal overflow at 320 CSS pixels, reduced-motion mismatch, or missing
accessible names in Chrome's accessibility tree.

Install the locked development dependencies with `npm ci`, start a local
bootstrapped server, then provide `A11Y_BASE_URL`, `A11Y_EMAIL`, and
`A11Y_PASSWORD` to `npm run test:a11y`; set `A11Y_LOCALE=en` or `es`. The runner
never prints the password.

### Audit history finding — 2026-11-16

An additional automated scan found one serious
`scrollable-region-focusable` violation on the audit history page, at
`#audit-table .table-wrap`. It used axe-core 4.14.0 with WCAG 2 A/AA and 2.1 AA
tags, Chrome 157.0.8103.78, a 390×844 viewport, Spanish locale and a dark
system preference. The scoped run and reproduction command are recorded in
[administration measurement evidence](ui-measurement-admin.md#state-and-attribution-checks).
[Issue #425](https://github.com/dynamiscode/dynamis-code-apps-template/issues/425)
owns the follow-up. The overflow region now has a keyboard tab stop and an
accessible name. The regression is covered by the audit route test. The audit
page axe smoke now passes in English and Spanish with zero violations. The
earlier results below remain historical observations and do not establish
current accessibility conformance. Automated scanning is not a full
accessibility audit.

The axe pass also covers the client list, a client page, the custom-fields page,
the files page, recipient notification center, public share, and MCP OAuth
consent. The public share and consent routes run with the multi-workspace and
OAuth settings enabled on the disposable smoke server. The files check audits
both locales, verifies the named upload progress control, and checks 320
CSS-pixel reflow. `make ux-smoke` creates a client, defines a choice field, and
enters a value.

## Manual checklist

| Check | Result | Evidence |
|---|---|---|
| Keyboard-only order and actions | Pass | Sign-in order reached the skip link, email, password, the reveal toggle, submit, then the password-reset link; native links, forms, selects, and buttons require no pointer |
| Focus order and visibility | Pass | DOM order is visual order; the two-color focus ring remained visible on light and colored controls |
| Forms and errors | Pass | Labels are programmatic; HTMX validation returns a `role=alert` target, focuses it after swap, and the import form supplies a required confirmation plus useful `role=status`/`role=alert` text |
| 200% zoom and reflow | Pass | Equivalent 320 CSS-pixel viewport had no horizontal page overflow; controls stack at the narrow breakpoint |
| Reduced motion | Pass | Emulated `prefers-reduced-motion: reduce` matched and disables animation, transition, and smooth scrolling |
| Contrast and target size | Pass | Text ratios: 15.01:1 body, 5.82:1 muted, 7.92:1 accent, 7.46:1 danger; controls have a 44 CSS-pixel minimum height |
| Screen reader | Pass | VoiceOver cursor traversed sign-in, workspace, and item flows in a disposable browser; Chrome AX-tree checks confirmed named headings, text fields, and buttons |

HTMX updates replace one named section, error alerts receive focus after a
swap, full-page errors receive focus on load, client validation focuses an error
summary that links to each invalid field, toasts use a polite live region, and
the realtime status uses a polite live region. Every page starts with a "Skip to
content" link. No accessibility
exception is open.

The workspace home keeps brand, workspace, and account controls. The workspace
sidebar shows `Home` above `Items` in the workspace context, with Settings separated by flexible space
and anchored at the bottom. Settings routes show `Members & invitations`,
`API tokens`, `Export`, and `Import`; owners and admins also see `Provisioning (SCIM)`
and `Audit history`.
The audit history uses a captioned table with scoped column headers and a
bounded overflow wrapper for narrow screens. The workspace home exposes Items and
Settings cards. In multi mode the Items page provides `Back to Workspaces`, returning to the
workspace selector; single mode omits it. Settings pages provide `Back to home`, returning to the
current workspace home.
The workspace switcher, account menu, and client controls use keyboard-accessible
native disclosure controls; narrow workspace and settings navigation use two-column grids
without horizontal overflow, and the workspace menu stays within the viewport. The item page
reports connecting, connected, reconnecting, or unavailable realtime states;
permanent deletion, member removal, ownership transfer, account deletion, and
every revoke confirm before submission when scripting is available.

Optional WebMCP preparation uses the same visible controls and focus ring as
ordinary keyboard interaction. `make webmcp-smoke` verifies focus after native
tool activation and confirms that preparation does not submit a mutation;
unsupported browsers use the ordinary-form fallback.
