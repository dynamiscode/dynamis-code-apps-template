---
name: form-wizard
description: Check multi-step forms and progressive disclosure - the two-step first-run setup wizard (Next only from a valid step, submit only from the last step with every field valid) and sections shown only for the chosen option (email provider) - with a TLA+ model, exhaustive tests of wizardMove, and browser checks. Use when a change touches data-wizard, data-step or data-show-when in internal/web/auth.templ and internal/web/settings_pages.templ, enhanceWizard, wizardMove, revealStep or enhanceDisclosure in the wireForms section of internal/web/assets/alpine-components.js, or the setup handlers in internal/web/handler.go.
---

# Form wizard and progressive disclosure

## The rules

1. **NextOnlyWhenValid.** You reach the workspace step only with a valid account step. Harm: the owner finds an account error only at the end, on a step they cannot see.
2. **SubmitOnlyWhenAllValid / SubmitOnlyFromLastStep.** The browser sends setup only from the last step, with every field on every step valid; an invalid field on a hidden step reopens that step.
3. **No skipped steps.** `wizardMove` moves at most one step, forward only from a valid step.
4. **Disclosure shows exactly the chosen section.** On email settings only the section for the selected provider shows; a hidden section never blocks a submit (validation skips hidden fields).
5. **Works without JavaScript.** Every step and section renders visible, and the server validates as before.

## What is modeled

- Model: `verification/web/SetupWizard.tla`, config `verification/web/SetupWizard.cfg`; 8 distinct states. Actions: fill or break each step, Next, Back, Create owner, and the server refusing setup (the page re-renders at step 1 with the error focused).
- `wizardMove` is tested over every step, total up to 6, direction and validity in `internal/web/assets/forms.test.mjs`.
- Browser checks (`internal/browsertest/ux_test.go`, `TestFormUXPatterns`): step 2 and the submit start hidden, Next is blocked on an invalid step, focus moves to the new step's legend, setup completes from the last step, and the email provider switch swaps sections.
- Left out: the setup token field's server check and the server's own validation rules.

## Coverage record

- Covered code: `internal/web/auth.templ` (setup wizard), `internal/web/settings_pages.templ` (email provider disclosure), `internal/web/assets/alpine-components.js`, wireForms (`wizardMove`, `enhanceWizard`, `revealStep`, `enhanceDisclosure`, the submit handler's wizard branch, `validateFields` skipping hidden fields).
- Sync commit: `14a5d42b3c5d96a7e72759e6c20c13a3e91b370f`. Diff the covered files against it before trusting a pass, then update this file.

## Run the check

1. `verification/tools/run-tlc.sh verification/web SetupWizard` → `Model checking completed. No error has been found.`
2. `node --test internal/web/assets/forms.test.mjs` → `fail 0`.
3. `go test -tags browser -count=1 -timeout=3m ./internal/browsertest -run '^TestFormUXPatterns$'` → `ok` (it starts from an empty database, so it walks setup).
4. Read a counterexample or redraw `docs/statecharts/setup-wizard/chart/chart.mmd` with the `/statechart` skill and `docs/statecharts/setup-wizard/view.js`.

## Read a failure

- **TLC** names an action property, for example `NextOnlyWhenValid`: the step that broke it is the last in the trace. Replay it on `/setup` with a fresh database.
- **forms.test.mjs `wizard:`**: `wizardMove` moved without a valid step or skipped one.
- **ux smoke `form-wizard: ...`**: the DOM wiring changed (hidden steps, button visibility, focus).
- Adding a step: add a `fieldset data-step` with a `legend`; extend the model only if the new step adds a rule.

## Proof the check catches a break

Measured on scratch changes, then reverted: `wizardMove` ignoring validity failed `forms.test.mjs`, and the model with an ungated Next failed `NextOnlyWhenValid`.
