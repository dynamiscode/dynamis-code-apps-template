# Dialogs, menus, onboarding, and forms

Choose the least disruptive pattern that supports the task. A full page is
usually easier to link to and offers more space; inline editing retains local
context; a dialog interrupts and should contain a bounded task. These are
trade-offs, not a mandate to use any one pattern.

## Dialogs and menus

- Name the dialog and its purpose; expose an obvious exit and preserve work
  appropriately. For a modal, move focus inside on opening, keep keyboard focus
  inside, support Escape, and restore focus on close when appropriate. Check the
  real implementation against W3C's modal pattern, including initial focus.
- A menu should expose actual actions with clear labels, keyboard operation,
  visible focus, and a way to close. Do not hide the primary task behind a menu
  without a reason. Test focus and non-JavaScript fallback for chosen behavior.
- Explain irreversible consequences and scope before commitment. Confirmations
  are not substitutes for authorization or safe defaults.

## Forms and first use

- Ask only for information needed; group related fields in a meaningful order,
  label controls persistently, and explain required formats before failure.
- Preserve entries after rejection; identify the specific problem and link
  summary to fields when appropriate. On success explain what happened and what
  comes next. Test retry, double submit, session expiry, and return paths when
  they matter.
- First-use and empty states should explain purpose, prerequisites, and one
  useful next action for the user's role. Keep necessary context available
  later; avoid forcing a tour when contextual help suffices.
- For multi-step tasks show the user's place and a safe way back when useful;
  preserve work and allow correction before consequential submission. Do not
  invent a progress percentage or assume a wizard is always better.

## Sources

[W3C modal dialog pattern](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/) ·
[GOV.UK validation](https://design-system.service.gov.uk/patterns/validation/) ·
[USWDS forms](https://designsystem.digital.gov/components/form/)
