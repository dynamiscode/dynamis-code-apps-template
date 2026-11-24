# Issue 656: public consent reflow review

**Partial review; identity and account journeys remain unreviewed.** Application revision `809e6f5`, plus the session-control styling in this change. Real local server, disposable SQLite workspace and owner, separate headless Chrome browser; synthetic consent and visitor name. Browser fixture cleans up its own server and browser. No shared Playwright session used.

## Public request journey

Owner signed in, created an open engagement stage, and enabled the request form with 1,430 unbroken synthetic characters. Visitor opened the real generated public link, focused the consent checkbox, entered a name, accepted consent, and submitted without email or phone. The page displayed the contact validation error. Both form and error states retained the consent and fit the viewport.

- 390×844 dark: document scroll width 375px (15px scrollbar); [form](mobile-dark-form.png), [error](mobile-dark-error.png). Earlier review measured 10,387px at 390px for a different long-consent fixture; this is not a controlled before/after measurement.
- 1440×900 light: document scroll width 1,425px (15px scrollbar); [form](desktop-light-form.png), [error](desktop-light-error.png).
- `TestPublicIntakeLongConsentReflow` checks rendered document width, full consent length, checkbox focus, error message, and axe violations on form and error states. Screenshots contain only synthetic data; the address bar and generated link are not captured.

## Three review perspectives

- Journey: contact requirement appears before fields; missing contact produces an error. Successful public submission was not exercised.
- Interaction and hierarchy: submit remains primary; consent wraps without widening the document. Checkbox can receive programmatic keyboard focus; full keyboard traversal was not reviewed.
- Accessibility and implementation: axe returned no violations for tested states. Automated checks do not establish accessibility conformance; screen-reader behavior and no-JavaScript fallback were not tested.

## Remaining gaps

This is not completion evidence for #656. Account, notifications, language, security, sessions, setup, sign-in, MFA, recovery, invitation, and optional authorization consent need rendered journey review, including success and failure, feature-gated states, keyboard navigation, and no-JavaScript fallback. Full source inventory across the union of origin/main and the execution base has not been recorded. Public submission success remains untested.
