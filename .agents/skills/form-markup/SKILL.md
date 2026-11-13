---
name: form-markup
description: Check every HTML template for the form markup rules - persistent labels, focusable error alerts for htmx swaps, confirmation on destructive buttons, and explicit optional marking - with known findings pinned. Use when a change adds or edits a form, input, select, textarea, role="alert" element, or a delete, remove, revoke or transfer button in internal/web/templates/, or changes the data-confirm or htmx:after:swap focus handlers in internal/web/assets/forms.js.
---

# Form markup

## The rules

1. **Persistent labels.** Every visible input, select and textarea has a label that stays on screen: `<label for=id>`, a wrapping `<label>`, or an `aria-label`. No placeholder-only fields. Harm: screen reader users cannot tell what a field is.
2. **Error auto-focus.** In a template htmx swaps (`hx-target`), every `role="alert"` has `tabindex="-1"`, so the `htmx:after:swap` handler in `forms.js` can move focus to it. forms.js also focuses the first full-page error on load. Harm: keyboard users never hear the error.
3. **Confirmation dialog.** Every button with value delete, remove, revoke or transfer has `data-confirm`, which `forms.js` turns into `window.confirm`. Harm: one stray click removes a member or revokes a token.
4. **Explicit optional marking.** A text field that is not `required` says "optional" in its label key. Selects are skipped because they always have a value.

## Known findings

None. `TestFormMarkupRules` keeps two empty maps for findings the user accepts later; it fails when a new break appears and when a listed one is fixed. Rule 4 skips fields inside a `data-show-when` section (progressive disclosure, see `form-wizard`), because those are required only for the option that shows the section.

## Why a template scan

The rules are about markup, and the template set is finite, so the check reads every embedded template (`files` in `handler.go`) and checks each tag. It covers every template, not a sample, but it reads template source, not rendered pages, so a control built only inside a `{{template}}` call from Go strings would be missed.

## Coverage record

- Covered: all of `internal/web/templates/*.html`; the `data-confirm` click handler and the `htmx:after:swap` focus handler in `internal/web/assets/forms.js`.
- Check: `internal/web/form_markup_test.go`.
- Sync commit: the commit that last changed this file (`git log -1 --format=%H -- .agents/skills/form-markup/SKILL.md`). If `forms.js` changed how confirm or focus works since then, update the rules here before trusting a pass. Move the sync commit only after that.

## Run the check

`go test ./internal/web/ -run TestFormMarkupRules -count=1`. Under two seconds.

## Read a failure

- `control without a persistent label: <tag>`: add a label for that control.
- `htmx-swapped alert cannot take focus`: add `tabindex="-1"` to the alert.
- `new break of "destructive button without data-confirm": members.html remove`: add `data-confirm` with a translated message. Add to the known list only with the user's approval.
- `known finding fixed, remove it from the list`: delete that entry from the map in the test.

## Proof the check catches a break

Measured at the sync commit on `item-list.html`, then reverted: removing the `new-title` label, the alert's `tabindex="-1"`, and the delete button's `data-confirm` each produced its own failure.
