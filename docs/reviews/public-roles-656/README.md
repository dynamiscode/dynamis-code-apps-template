# Issue 656: public consent reflow review

**Partial review; feature-gated journeys remain unreviewed.** Application revision `f223ac5`, plus invitation styling and browser tests in this change. Real local server, disposable SQLite workspace and owner, separate headless Chrome browser; synthetic consent and visitor name. Browser fixture cleans up its own server and browser. No shared Playwright session used.

## Public request journey

Owner signed in, created an open engagement stage, and enabled the request form with 1,430 unbroken synthetic characters. Visitor opened the real generated public link, focused the consent checkbox, entered a name, accepted consent, and submitted without email or phone. The page displayed the contact validation error. Both form and error states retained the consent and fit the viewport.

- 390×844 dark: document scroll width 375px (15px scrollbar); [form](mobile-dark-form.png), [error](mobile-dark-error.png). Earlier review measured 10,387px at 390px for a different long-consent fixture; this is not a controlled before/after measurement.
- 1440×900 light: document scroll width 1,425px (15px scrollbar); [form](desktop-light-form.png), [error](desktop-light-error.png).
- `TestPublicIntakeLongConsentReflow` checks rendered document width, full consent length, checkbox focus, error message, and axe violations on form and error states. Screenshots contain only synthetic data; the address bar and generated link are not captured.

## Additional browser journeys

At 390×844 light with JavaScript disabled, a visitor submitted a name, synthetic email, and consent; the real application displayed [confirmation](mobile-nojs-success.png). A separate owner entered an incorrect password and received an error, then signed in successfully, opened [account](mobile-nojs-account.png), and followed Sessions. The [sign-in screen](mobile-nojs-login.png), account and sessions had no document overflow. These are synthetic fixtures; no credentials or private links appear in captures.

`TestAccountAndSecurityPagesRender` additionally visited account, language, security and sessions at 1280×900 and 390×844 in both themes, and checked narrow reflow. `TestProfilePreferencePersistsThroughOrdinaryForm` saved a profile change. These automated visits are not a visual review of every state.

## Source inventory

Compared names changed between local `origin/main` and this branch and inspected account, authentication, security, sessions, notifications, invitations, public consent and request-form templates. Account shortcuts, recovery and provider links, notifications and public return links use outlined navigation; form submissions remain native buttons. Invitation resend now has a secondary outline and revoke has a danger outline; its existing confirmation checkbox remains. Shared account navigation is supplied by `components/organisms/navigation.templ`. Setup, MFA, recovery and email verification use shared auth layout and action components; OAuth consent uses native allow/deny buttons. This is a scoped source inventory, not proof of all feature-gated rendering.

## Three review perspectives

- Journey: contact requirement appears before fields; missing contact produces an error; valid submission reaches confirmation without JavaScript.
- Interaction and hierarchy: submit remains primary; consent wraps without widening the document. Checkbox can receive programmatic keyboard focus; full keyboard traversal was not reviewed.
- Accessibility and implementation: axe returned no violations for tested public form and error states. Automated checks do not establish accessibility conformance; screen-reader behavior was not tested.

## Remaining gaps

This is not completion evidence for #656. Notifications, setup, MFA, recovery, invitation and optional authorization consent still need rendered journey review, including feature-gated states and keyboard navigation. Full keyboard traversal, screen-reader review, and successful invitation or OAuth consent were not exercised. Source comparison used the locally available `origin/main` ref; no remote refresh was performed.
