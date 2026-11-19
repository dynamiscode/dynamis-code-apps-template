---
name: form-markup
description: Check every templ form for the form markup rules - persistent labels, focusable error alerts for htmx swaps, confirmation on destructive buttons, and explicit optional marking. Use when a change adds or edits a form, input, select, textarea, role="alert" element, or a delete, remove, revoke or transfer button in internal/web/*.templ, or changes the data-confirm or htmx:after:swap focus handlers in internal/web/assets/forms.js.
---

# Form markup

## The rules

1. **Persistent labels.** Every visible input, select and textarea has a label that stays on screen: `<label for=id>`, a wrapping `<label>`, or an `aria-label`. No placeholder-only fields. Harm: screen reader users cannot tell what a field is.
2. **Error auto-focus.** In a template htmx swaps (`hx-target`), every `role="alert"` has `tabindex="-1"`, so the `htmx:after:swap` handler in `forms.js` can move focus to it. forms.js also focuses the first full-page error on load. Harm: keyboard users never hear the error.
3. **Confirmation dialog.** Every button with value delete, remove, revoke or transfer has `data-confirm`, which `forms.js` turns into `window.confirm`. Harm: one stray click removes a member or revokes a token. Some forms confirm with a required `confirm_*` checkbox instead (members, webhooks); that also counts.
4. **Explicit optional marking.** A text field that is not `required` says "optional" in its label key. Selects are skipped because they always have a value. Fields inside a `data-show-when` section are required only for the option that shows the section (see `form-wizard`).

## Enforcement

No automated source scan. `TestFormMarkupRules` read the legacy `templates/*.html` files, which were removed when the renderer moved to templ. Its regexes do not understand templ syntax (`required/>`, dynamic `id={ ... }`, caller-supplied labels in components), so a replacement needs its own design. Until then, review these rules by hand on every form change and run the browser checks (`make ux-smoke`, `make accessibility-smoke`).

## Coverage record

- Covered by review: all `internal/web/*.templ` forms; the `data-confirm` click handler and the `htmx:after:swap` focus handler in `internal/web/assets/forms.js`.
- Sync commit: the commit that last changed this file (`git log -1 --format=%H -- .agents/skills/form-markup/SKILL.md`). If `forms.js` changed how confirm or focus works since then, update the rules here before trusting a pass.

## Proof a break is caught by hand

Remove the `new-title` label, the alert's `tabindex="-1"`, or a delete button's `data-confirm` in a template, then confirm the relevant browser or accessibility check fails. Revert afterwards.
