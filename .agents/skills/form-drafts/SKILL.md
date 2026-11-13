---
name: form-drafts
description: Check how forms keep unsent work - dirty detection that enables Save only after a change, the unsaved-changes warning, sessionStorage drafts restored after a rejected submit or a return visit, and realtime list refreshes that wait while you edit - with a TLA+ model, exhaustive evaluator tests, and browser checks. Use when a change touches data-persist, data-dirty-track, data-requires-change or data-unsaved-warning in internal/web/templates/, the draft, dirty, beforeunload or isEditing code in internal/web/assets/forms.js, or the realtime refresh in internal/web/assets/app.js.
---

# Form drafts

## The rules

1. **KeptOnReject.** After the server rejects a submit, the field shows what you typed (restored from the draft, or echoed by the server). Harm: retyping a whole form after one mistake.
2. **NoStaleDraft.** After the server saves, the draft is gone and never comes back. Harm: old text reappears and gets submitted twice.
3. **DraftSurvivesLeaving.** Coming back to a form shows unsent text until it is saved or discarded.
4. **NoUnwarnedLoss / WarnOnlyWhenDirty.** Leaving with unsaved text in a `data-unsaved-warning` form warns; a clean form never warns.
5. **Save enabled only after a change** (`data-requires-change`), and **a realtime refresh never replaces a row you are editing** (`refreshDecision`, `isEditing`).

## What is modeled

- Model: `verification/web/FormDrafts.tla`, config `verification/web/FormDrafts.cfg`; 29 distinct states. One full-page form whose server re-render does not echo input (the invitation form).
- Actions → forms.js: `YouType`/`YouClear` (input handler: `updateDirty`, `writeDraft(form, false)`), `YouSubmit` (submit handler: `writeDraft(form, true)`, then `submitting`), `EventWhileSubmitting` (an input/change on the old form during the request; `writeDraft` skips a submitting form), `ServerSaves`/`ServerRejects`/`YouComeBack` (page load: `applyDraft` → `draftAction`), `YouLeave` (`beforeunload`), `YouDiscard` (the Discard changes button).
- Exhaustive tests (`internal/web/assets/forms.test.mjs`): `draftAction` over every stored/submitted/error combination, `isDirty` over every pair of two-field values, `refreshDecision`.
- Browser checks (`scripts/ux-patterns.mjs`, lines tagged `form-drafts`): Save starts disabled and enables after a change, the invitation draft is restored and announced, Discard clears it, the warning fires only when dirty.
- Left out: several tabs (sessionStorage is per tab), text typed while a request is in flight (the response replaces the form), and the realtime refresh timing (only the decision is tested; `app.js` applies it on `focusout` and after each request).

## Coverage record

- Covered code: `internal/web/assets/forms.js` (`draftAction`, `isDirty`, `refreshDecision`, `writeDraft`, `readDraft`, `applyDraft`, `updateDirty`, the input/change/submit/beforeunload handlers, `isEditing`), `internal/web/assets/app.js` (the deferred `refresh`), and the templates using `data-persist` (item-list create form, invitations, webhooks), `data-dirty-track` (item rows, account profile, general settings, email settings), `data-unsaved-warning`.
- Sync commit: the commit that last changed this file (`git log -1 --format=%H -- .agents/skills/form-drafts/SKILL.md`). Diff the covered files against it first; if they changed, confirm the model still matches before trusting a pass, then update this file.

## Run the check

1. `verification/tools/run-tlc.sh verification/web FormDrafts` → `Model checking completed. No error has been found.`
2. `node --test internal/web/assets/forms.test.mjs` → `fail 0`.
3. `make ux-smoke` → `N UX checks passed` (needs Chrome and `npm ci`).
4. Read a counterexample, or redraw `docs/statecharts/form-drafts/chart/chart.mmd`, with the `/statechart` skill on the model and `docs/statecharts/form-drafts/view.js`.

## Read a failure

- **TLC:** the counterexample is a sequence of user and server steps. Replay it in a browser on the invitations page. If the page does the same, it is a bug; do not weaken the rule. If the code cannot take a step, fix the model.
- **forms.test.mjs:** `draft decision` names the stored/error combination that picked the wrong action.
- **ux smoke:** `not ok - form-drafts: ...` names the rule; `item-create-submission: the box is empty after a save` also means a stale draft was restored.

## Proof the check catches a break

Measured on scratch changes, then reverted:
- `writeDraft` without the submitting guard: `make ux-smoke` fails (`the box is empty after a save`); the model variant where `EventWhileSubmitting` writes the draft fails `NoStaleDraft`. This guard came from a real bug the browser run found.
- `draftAction` never returning "discard": `forms.test.mjs` fails; the model variant fails `NoStaleDraft`.
