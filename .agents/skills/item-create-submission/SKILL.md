---
name: item-create-submission
description: Check the item create form's submit lifecycle (busy button while sending, one item per press, rejected titles kept, no silently lost press) with a TLA+ model and a Go replay test. Use when a change touches the create form in internal/web/templates/item-list.html (idempotency_key, hx-post, hx-sync, the new-title value), createItem, renderItems, itemError, itemToastKey or afterMutation in internal/web/handler.go, Create or create in internal/items/service.go, the submit/busy code in internal/web/assets/forms.js, htmx.config in app.js, or the vendored htmx.min.js.
---

# Item create submission

## The rules (all hold)

1. **OneItemPerKey.** One Add press never makes two items, even when resent. Harm: duplicate items from double clicks or retries.
2. **NoSilentLoss.** Every Add with a real title is stored, or the page says it was not. Harm: the user thinks an item was saved when it was dropped.
3. **InputKeptOnReject.** After the server rejects a title (blank, item limit, reused form key), the box still shows it. Harm: retyping after one mistake.
4. **BusyWhileSending.** While an Add is on its way, the button is disabled and shows "Saving". Harm: no feedback, so users press again.
5. **OneRequestInFlight.** At most one Add is in flight from the form.

## What is modeled

- Model: `verification/web/ItemCreate.tla`, config `verification/web/ItemCreate.cfg`. Bounds: titles {A, B} and a blank title, item limit 2, three presses; about 830 distinct states, under a second.
- Actions → code: `YouType`; `YouPressAdd` (forms.js submit handler marks the form submitting at once and cancels a second submit; `hx-sync="this:drop"` backs it up); `ServerAnswers` (one `create()` transaction: idempotency lookup, limit, insert); `RequestFails` (5xx or network: htmx does not swap, forms.js clears busy and shows the "Something went wrong" toast); `ResponseArrives` (swap: new form and key; empty box after a save, `NewTitle` echoed after a reject; toast via `HX-Trigger: app:toast`).
- Left out: the no-JavaScript path (plain POST; the same server rules apply), edits and deletes (see `async-feedback`), realtime refreshes (see `form-drafts`), idempotency expiry (24 h), and text typed while a request is in flight (the swap replaces the box).

## Coverage record

- Covered code: `internal/web/templates/item-list.html` (create form), `internal/web/handler.go` (`createItem`, `afterMutation`, `renderItems`, `itemError`, `itemToastKey`, `render` HX-Trigger), `internal/items/service.go` (`Create`, `create`, `validateTitle`), `internal/web/assets/forms.js` (submit handler, `setBusy`, `htmx:finally:request`), `internal/web/assets/app.js` (`htmx.config.noSwap`), `internal/web/assets/htmx.min.js` (`hx-sync` drop).
- Checks: the model; `internal/web/item_create_model_test.go` (`TestItemCreateModelSteps`, one subtest per server or page fact); the item lines in `scripts/ux-patterns.mjs` (busy button, one item per double press, empty box after save).
- Sync commit: the commit that last changed this file (`git log -1 --format=%H -- .agents/skills/item-create-submission/SKILL.md`). Run `git diff <sync> -- <covered files>`; if covered code changed, confirm the model still matches before trusting a pass. Update this file (and so the sync commit) only after that.

## Run the check

1. `verification/tools/run-tlc.sh verification/web ItemCreate` → `Model checking completed. No error has been found.`
2. `go test ./internal/web/ -run TestItemCreateModelSteps -count=1` → `ok`.
3. For browser changes: `make ux-smoke` → `N UX checks passed` (needs Chrome and `npm ci`).
4. Read any counterexample, or redraw the chart, with the `/statechart` skill on `verification/web/ItemCreate.tla`, config `docs/statecharts/item-create/ItemCreate.cfg`, view `docs/statecharts/item-create/view.js`. Committed chart: `docs/statecharts/item-create/chart/chart.mmd`.

## Read a failure

- **TLC breaks a rule:** replay each step against the code above, one action per step. If the code can do it, it is a real bug; report the rule and steps, and do not weaken the rule. If the code cannot take a step, fix the model.
- **A replay subtest fails:** the subtest name is the model action whose fact changed, for example `reused key, new title = 500` means the `ErrIdempotencyConflict` case left `itemError`. Update the action, rerun TLC.
- **`make ux-smoke` fails** with `not ok - async-feedback: the Add button is disabled while sending` or `item-create-submission: ...`: the browser half changed; read the forms.js diff.

## Proof the check catches a break

Measured on scratch changes, then reverted:
- `itemError` without the `ErrIdempotencyConflict` case: `TestItemCreateModelSteps` fails (`reused key, new title = 500`).
- `create()` hashing a per-request key: the replay test fails (`Alpha shown 2 times`).
- forms.js without `setBusy(form, true)`: `make ux-smoke` fails (`the Add button is disabled while sending`); the model with `busy' = FALSE` fails `BusyWhileSending`.
- The model without the idempotency lookup fails `OneItemPerKey`.
