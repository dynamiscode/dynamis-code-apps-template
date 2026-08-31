# Accessibility Evidence

Standard: WCAG 2.2 Level AA  
Review date: 2026-08-25  
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

## Manual checklist

| Check | Result | Evidence |
|---|---|---|
| Keyboard-only order and actions | Pass | Sign-in order reached email, password, then submit; native links, forms, selects, and buttons require no pointer |
| Focus order and visibility | Pass | DOM order is visual order; the two-color focus ring remained visible on light and colored controls |
| Forms and errors | Pass | Labels are programmatic; HTMX validation returns a `role=alert` target, focuses it after swap, and supplies useful text |
| 200% zoom and reflow | Pass | Equivalent 320 CSS-pixel viewport had no horizontal page overflow; controls stack at the narrow breakpoint |
| Reduced motion | Pass | Emulated `prefers-reduced-motion: reduce` matched and disables animation, transition, and smooth scrolling |
| Contrast and target size | Pass | Text ratios: 15.01:1 body, 5.82:1 muted, 7.92:1 accent, 7.46:1 danger; controls have a 44 CSS-pixel minimum height |
| Screen reader | Pass | VoiceOver cursor traversed sign-in, workspace, and item flows in a disposable browser; Chrome AX-tree checks confirmed named headings, text fields, and buttons |

HTMX updates replace one named section, error alerts receive focus after a
swap, and the realtime status uses a polite live region. No accessibility
exception is open.

The workspace home keeps brand, workspace, and account controls. The workspace
sidebar shows `Home` above `Items` in the workspace context, with Settings separated by flexible space
and anchored at the bottom. Settings routes show `Members & invitations`,
`API tokens`, and `Export`; owners and admins also see `Audit history`.
The audit history uses a captioned table with scoped column headers and a
bounded overflow wrapper for narrow screens. The workspace home exposes Items and
Settings cards. The Items page provides `Back to Workspaces`, returning to the
workspace selector. Settings pages provide `Back to home`, returning to the
current workspace home.
The workspace switcher and account menu use keyboard-accessible native disclosure controls;
narrow screens stack the same links without horizontal overflow. The item page
reports connecting, connected, reconnecting, or unavailable realtime states;
permanent deletion confirms before submission when scripting is available.

Optional WebMCP preparation uses the same visible controls and focus ring as
ordinary keyboard interaction. `make webmcp-smoke` verifies focus after native
tool activation and confirms that preparation does not submit a mutation;
unsupported browsers use the ordinary-form fallback.
